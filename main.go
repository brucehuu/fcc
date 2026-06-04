package main

import (
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
)

const version = "0.1.0"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [workdir]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "feishu-connect - 本机终端与飞书的双向实时桥接服务\n\n")
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
		fmt.Printf("feishu-connect %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load("env")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	log.SetLevel(cfg.LogLevel)

	// 解析命令行参数：可选的项目路径
	workDir := ""
	if args := flag.Args(); len(args) > 0 {
		workDir = args[0]
		if st, err := os.Stat(workDir); err != nil || !st.IsDir() {
			fmt.Fprintf(os.Stderr, "invalid working directory: %s\n", workDir)
			os.Exit(1)
		}
	}

	log.Infof("[main] starting feishu-connect with command: %s", cfg.Command)
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

	// 处理中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("[main] shutting down...")
		cancel()
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

	// 前台 attach tmux，用户可以在电脑终端看到完整界面
	fmt.Println()
	fmt.Println("[main] attaching to tmux session...")
	fmt.Println("  Press Ctrl+B then D to detach (飞书仍保持同步)")
	fmt.Println("  Press Ctrl+C to stop the program")
	fmt.Println()

	cmd := exec.Command("tmux", "attach", "-t", "feishu-connect")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Warnf("[main] tmux attach exited: %v", err)
	}

	fmt.Println("[main] detached from tmux. Press Ctrl+C to stop or re-attach with: tmux attach -t feishu-connect")

	<-ctx.Done()

	// 先关闭 messenger 加速 goroutine 退出
	b.Close()

	// 输出运行时指标
	b.LogMetrics()

	// 优雅关闭：等待所有 goroutine 退出，最多 5 秒
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := b.Shutdown(shutdownCtx); err != nil {
		log.Warnf("[main] shutdown incomplete: %v", err)
	}
	// tmux session 保留：用户可以重新 attach，也可以手动 kill
}
