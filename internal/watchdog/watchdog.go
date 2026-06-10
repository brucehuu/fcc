package watchdog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"fcc/internal/log"
)

var (
	watchdogPidFile = "/tmp/fcc-watchdog.pid"
	fccPidFile      = "/tmp/fcc.pid"
	checkInterval   = 6 * time.Second

	// Injectable dependencies for testing.
	execCommandFunc          = exec.Command
	isFCCRunningFunc         = isFCCRunning
	isTmuxSessionRunningFunc = isTmuxSessionRunning
	startFCCFunc             = startFCC
)

// SetCheckInterval 允许外部调整检查间隔（如从配置读取）。
func SetCheckInterval(d time.Duration) {
	if d > 0 {
		checkInterval = d
	}
}

// ForkIfNeeded 检查是否已有 watchdog 在运行，没有则 fork 一个。
// 在正常 fcc 模式启动时调用。
func ForkIfNeeded() error {
	if isWatchdogRunning() {
		log.Info("[watchdog] already running, skipping")
		return nil
	}

	// 用文件锁防止多个 fcc 同时 fork watchdog（竞态）
	lockFile := "/tmp/fcc-watchdog.lock"
	fd, err := syscall.Open(lockFile, syscall.O_CREAT|syscall.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer syscall.Close(fd)

	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Info("[watchdog] another fcc is forking, skipping")
		return nil
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)

	// 获取锁后再次检查（防 TOCTOU 竞态）
	if isWatchdogRunning() {
		log.Info("[watchdog] already running (after lock), skipping")
		return nil
	}

	return forkWatchdog()
}

// Run 运行 watchdog 主循环。在 WATCHDOG=1 模式下调用。
func Run() error {
	return RunWithContext(context.Background())
}

// RunWithContext 运行 watchdog 主循环，支持通过 context 取消。
func RunWithContext(ctx context.Context) error {
	pidStr := fmt.Sprintf("%d", os.Getpid())

	if err := os.WriteFile(watchdogPidFile, []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("write pid file failed: %w", err)
	}

	// 防竞态：再次读取确认是自己的 PID
	data, _ := os.ReadFile(watchdogPidFile)
	if string(data) != pidStr {
		log.Info("[watchdog] another instance won, exiting")
		return nil
	}

	log.Infof("[watchdog] started, checking every %v", checkInterval)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			fccRunning := isFCCRunningFunc()
			tmuxRunning := isTmuxSessionRunningFunc()

			if !fccRunning {
				log.Warn("[watchdog] fcc not running, restarting...")
				if err := startFCCFunc(); err != nil {
					log.Errorf("[watchdog] restart failed: %v", err)
				}
				continue
			}

			if !tmuxRunning {
				log.Warn("[watchdog] tmux session gone but fcc still alive, killing fcc to restart...")
				killFromPIDFile(fccPidFile)
				if err := os.Remove(fccPidFile); err != nil {
					log.Debugf("[watchdog] remove fcc pid file: %v", err)
				}
				// 下一轮循环会检测到 fcc 不在并重新启动
			}
		}
	}
}

// WriteFCCPID 写入 fcc 进程 PID 到文件。
func WriteFCCPID() error {
	return os.WriteFile(fccPidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

// RemoveFCCPID 删除 fcc 的 PID 文件。
func RemoveFCCPID() {
	if err := os.Remove(fccPidFile); err != nil {
		log.Debugf("[watchdog] remove fcc pid file: %v", err)
	}
}

func isWatchdogRunning() bool {
	data, err := os.ReadFile(watchdogPidFile)
	if err != nil {
		return false
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return false
	}

	return processExists(pid)
}

func forkWatchdog() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path failed: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir failed: %w", err)
	}

	cmd := execCommandFunc(exe)
	cmd.Dir = wd
	cmd.Env = append(os.Environ(), "WATCHDOG=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("fork watchdog failed: %w", err)
	}

	log.Infof("[watchdog] forked (pid: %d)", cmd.Process.Pid)
	return nil
}

func isFCCRunning() bool {
	data, err := os.ReadFile(fccPidFile)
	if err != nil {
		return false
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return false
	}

	return processExists(pid)
}

func isTmuxSessionRunning() bool {
	out, err := execCommandFunc("tmux", "has-session", "-t", "fcc").CombinedOutput()
	if err != nil {
		log.Debugf("[watchdog] tmux session check failed: %v, output: %s", err, string(out))
		return false
	}
	return true
}

// IsFCCRunning returns true if the fcc main process is currently running.
func IsFCCRunning() bool { return isFCCRunning() }

func startFCC() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path failed: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir failed: %w", err)
	}

	// 打开日志文件，把 fcc 的 stderr 重定向过来，方便排查启动失败
	_ = os.MkdirAll("log", 0755)
	logFile, err := os.OpenFile("log/fcc-restart.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	needClose := err == nil
	if err != nil {
		logFile = os.Stderr
	}

	// 清理可能残留的 tmux 会话，否则 fcc 启动会因会话已存在而失败
	if err := execCommandFunc("tmux", "kill-session", "-t", "fcc").Run(); err != nil {
		log.Debugf("[watchdog] kill tmux session: %v", err)
	}

	cmd := execCommandFunc(exe)
	cmd.Dir = wd
	// 关键：必须去掉 WATCHDOG=1，否则启动的还是 watchdog 模式
	cmd.Env = stripEnv(os.Environ(), "WATCHDOG")
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := cmd.Start(); err != nil {
		if needClose {
			_ = logFile.Close()
		}
		return fmt.Errorf("start fcc failed: %w", err)
	}
	// 子进程已通过 dup 继承了自己的 fd，父进程可以安全关闭
	if needClose {
		if err := logFile.Close(); err != nil {
			log.Debugf("[watchdog] close log file: %v", err)
		}
	}

	log.Infof("[watchdog] restarted fcc (pid: %d)", cmd.Process.Pid)
	return nil
}

// processExists 使用 ps 命令检查进程是否存活（macOS 上比 Signal(0) 更可靠）
func processExists(pid int) bool {
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "pid=").Output()
	if err != nil {
		return false
	}
	var found int
	_, err = fmt.Sscanf(string(out), "%d", &found)
	return err == nil && found == pid
}

// stripEnv 从环境变量中移除指定 key
func stripEnv(env []string, key string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// Reset 杀掉所有 fcc 相关进程（主进程 + watchdog），清理 PID 文件。
// 用于用户主动重启、系统更新等场景。幂等：无进程时安全。
// WATCHDOG 模式下不应调用此函数（会自杀）。
func Reset() {
	log.Info("[watchdog] reset: killing all fcc processes")

	// 1. 先通过 PID 文件精准 kill（SIGTERM 优雅退出）
	killFromPIDFile(watchdogPidFile)
	killFromPIDFile(fccPidFile)

	// 2. 兜底：杀掉从当前可执行文件路径启动的所有 fcc 进程（防止有漏网的）
	if exe, err := os.Executable(); err == nil {
		if err := execCommandFunc("pkill", "-9", "-f", exe).Run(); err != nil {
			log.Debugf("[watchdog] pkill fallback: %v", err)
		}
	}

	// 3. 清理残留的 tmux 会话
	if err := execCommandFunc("tmux", "kill-session", "-t", "fcc").Run(); err != nil {
		log.Debugf("[watchdog] kill tmux session: %v", err)
	}

	// 4. 删除所有 PID 文件和锁文件
	if err := os.Remove(watchdogPidFile); err != nil {
		log.Debugf("[watchdog] remove pid file: %v", err)
	}
	if err := os.Remove(fccPidFile); err != nil {
		log.Debugf("[watchdog] remove fcc pid file: %v", err)
	}
	if err := os.Remove("/tmp/fcc-watchdog.lock"); err != nil {
		log.Debugf("[watchdog] remove lock file: %v", err)
	}

	// 5. 等待进程退出
	time.Sleep(1 * time.Second)

	log.Info("[watchdog] reset done")
}

// Stop 终止 watchdog 进程，让 fcc 退出后不会被自动拉起。
// 用户主动 Quit fcc 时调用：先发 SIGTERM 等最多 1 秒，再清 PID 文件。
// 幂等：watchdog 不在时安全返回。
func Stop() {
	data, err := os.ReadFile(watchdogPidFile)
	if err != nil {
		return
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return
	}
	if !processExists(pid) {
		if err := os.Remove(watchdogPidFile); err != nil {
			log.Debugf("[watchdog] remove pid file: %v", err)
		}
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		log.Warnf("[watchdog] stop: signal: %v", err)
	}
	for i := 0; i < 20; i++ {
		if !processExists(pid) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := os.Remove(watchdogPidFile); err != nil {
		log.Debugf("[watchdog] remove pid file: %v", err)
	}
	log.Info("[watchdog] stopped")
}

// killFromPIDFile 读取 PID 文件，发送 SIGTERM 信号。如果进程不存在则跳过。
func killFromPIDFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return
	}
	if processExists(pid) {
		proc, err := os.FindProcess(pid)
		if err == nil {
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				log.Debugf("[watchdog] signal pid %d: %v", pid, err)
			}
		}
	}
}
