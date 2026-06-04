package main

import (
	_ "embed"

	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"feishu-connect/internal/bridge"
	"feishu-connect/internal/config"
	"feishu-connect/internal/log"
	"feishu-connect/internal/tray"
	"feishu-connect/internal/watchdog"
)

//go:embed assets/fcc-logo.png
var appIconPNG []byte

const version = "0.1.001"

func processIcon(src []byte) []byte {
	icon := src
	if removed, ok := tray.RemoveWhiteBackground(icon); ok {
		icon = removed
	}
	if padded, ok := tray.AddIconPadding(icon, 18); ok {
		icon = padded
	}
	if rounded, ok := tray.ApplyRoundedCorners(icon); ok {
		icon = rounded
	}
	return icon
}

func isFCCRunning() bool {
	data, err := os.ReadFile("/tmp/fcc.pid")
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return false
	}
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "pid=").Output()
	if err != nil {
		return false
	}
	var found int
	_, err = fmt.Sscanf(string(out), "%d", &found)
	return err == nil && found == pid
}

func main() {
	// --config-window 模式：helper 子进程跑 webview 配置窗口。
	// 需要在 Dock 显示 fcc 图标，所以提前处理图标并传给 RunConfigWindow。
	if len(os.Args) > 1 && os.Args[1] == "--config-window" {
		iconPNG := processIcon(appIconPNG)
		tray.RunConfigWindow(iconPNG)
		return
	}

	// watchdog 模式：尽早进入，跳过业务初始化和 Reset
	if os.Getenv("WATCHDOG") == "1" {
		log.SetLevel("info")
		if err := watchdog.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "watchdog: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 每次正常启动都先杀干净旧进程（主进程 + watchdog），然后重新启动
	log.SetLevel("info")
	watchdog.Reset()

	// macOS 上尽早初始化 NSApp：菜单栏 app 模式（不显示 Dock 图标）。
	// 必须在 systray.Run 之前调用。
	tray.SetupMainApp()

	// 给 fcc 可执行文件自身设置 Finder 图标（幂等，失败不阻塞启动）。
	if exe, err := os.Executable(); err == nil {
		_ = tray.SetFinderIcon(exe, processIcon(appIconPNG))
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [workdir]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "fcc - 本机终端与飞书的双向实时桥接服务\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s                          # 启动服务\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s /path/to/project         # 指定工作目录启动\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -v                       # 显示版本\n", os.Args[0])
	}

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.BoolVar(&showVersion, "v", false, "show version")
	flag.Parse()

	if showVersion {
		fmt.Printf("fcc %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(".env")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	log.SetLevel(cfg.LogLevel)

	// 启动 watchdog（完全独立进程）
	if err := watchdog.ForkIfNeeded(); err != nil {
		log.Warnf("[main] fork watchdog: %v", err)
	}

	// 解析命令行参数：可选的项目路径
	workDir := ""
	if args := flag.Args(); len(args) > 0 {
		workDir = args[0]
		if st, err := os.Stat(workDir); err != nil || !st.IsDir() {
			fmt.Fprintf(os.Stderr, "invalid working directory: %s\n", workDir)
			os.Exit(1)
		}
	}

	log.Infof("[main] starting fcc with command: %s", cfg.Command)
	if workDir != "" {
		log.Infof("[main] working directory: %s", workDir)
	}

	b, err := bridge.New(&bridge.BridgeConfig{
		AppID:             cfg.AppID,
		AppSecret:         cfg.AppSecret,
		Command:           cfg.Command,
		WorkDir:           workDir,
		BypassPermissions: cfg.BypassPermissions,
		CodexQueueMode:    cfg.CodexQueueMode,
		CaptureInterval:   cfg.CaptureInterval,
		SendTimeout:       cfg.SendTimeout,
		TMUXHistoryLines:  cfg.TMUXHistoryLines,
		SendRetries:       cfg.SendRetries,
		NoisePatterns:     cfg.NoisePatterns,
	})
	if err != nil {
		log.Errorf("failed to create bridge: %v", err)
		os.Exit(1)
	}
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 信号触发：发信号时调 tray.Stop() 让 NSApp 走 terminate 流程，
	// 然后 cfg.OnExit 会清理 tmux + cancel。**不动 watchdog**——
	// watchdog 要继续在，万一 fcc 之后被外部拉起还能监控。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("[main] received signal, shutting down...")
		tray.Stop()
	}()

	// 后台启动 bridge（飞书连接 + 输出捕获 + 图片清理）
	go b.Start(ctx)

	// 等待 bridge 和 tmux 初始化完成
	log.Info("[main] waiting for tmux session to start...")
	if err := b.WaitReady(); err != nil {
		log.Errorf("tmux not ready: %v", err)
		b.Close()
		os.Exit(1)
	}

	// 写入 PID 文件，供 watchdog 监控
	if err := watchdog.WriteFCCPID(); err != nil {
		log.Warnf("[main] write pid file: %v", err)
	}
	defer watchdog.RemoveFCCPID()

	// SIGUSR1: 配置页面保存后触发，热重启 tmux
	reloadCh := make(chan os.Signal, 1)
	signal.Notify(reloadCh, syscall.SIGUSR1)
	defer signal.Reset(syscall.SIGUSR1)
	go func() {
		for range reloadCh {
			log.Info("[main] received SIGUSR1, restarting tmux...")
			if err := b.RestartTmux(workDir); err != nil {
				log.Errorf("[main] restart tmux failed: %v", err)
			}
		}
	}()

	// tmux attach 必须在子 goroutine 跑 — 主线程要交给 tray.Run 跑 NSApp loop。
	cmd := exec.Command("tmux", "attach", "-t", "fcc")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println()
	fmt.Println("[main] fcc ready — tmux attaching in background")
	fmt.Println("  Press Ctrl+B then D to detach (fcc keeps running in menu bar)")
	fmt.Println("  Press Ctrl+C, or click 'Quit fcc' in the menu bar, to exit")
	fmt.Println()

	go func() {
		if err := cmd.Run(); err != nil {
			log.Warnf("[main] tmux attach exited: %v", err)
		}
		log.Info("[main] tmux detached; fcc continues in menu bar")
	}()

	// 阻塞主线程：跑 NSApp loop 直到用户 Quit。
	// cfg.OnExit：通用清理（杀 tmux + cancel），Ctrl+C 和菜单 Quit 都会走这里。
	// cfg.OnMenuQuit：只在用户点菜单 Quit 时跑，**杀 watchdog**（彻底退出）。
	tray.Run(tray.Config{
		Version:      version,
		OnOpenConfig: tray.OpenConfig,
		OnExit: func() {
			log.Info("[main] shutting down...")
			cancel()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		},
		OnMenuQuit: func() {
			log.Info("[main] menu quit requested — killing watchdog and tmux session")
			watchdog.Stop()
			// 把 tmux session 一起干掉，避免孤儿进程
			_ = exec.Command("tmux", "kill-session", "-t", "fcc").Run()
		},
	})

	// NSApp 已返回，做最终清理
	b.Close()
	b.LogMetrics()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := b.Shutdown(shutdownCtx); err != nil {
		log.Warnf("[main] shutdown incomplete: %v", err)
	}
	// tmux session 保留：用户可以重新 attach，也可以手动 kill
}
