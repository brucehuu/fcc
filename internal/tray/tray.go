//go:build darwin

package tray

import (
	"sync/atomic"

	"github.com/getlantern/systray"
)

const (
	menuTitleConfig = "Open Config"
	menuTitleExit   = "Quit fcc"
	tooltipText     = "fcc — Feishu Connect"
)

type Config struct {
	Version      string
	Icon         []byte
	OnOpenConfig func()
	OnExit       func()
	OnMenuQuit   func()
}

var started atomic.Bool

// Run 启动 macOS 菜单栏并阻塞当前 goroutine。**必须从主 goroutine 调用**。
//
// 内部走 systray.Run：先 Register（在 applicationDidFinishLaunching 里创建
// NSStatusItem），再 nativeLoop 启动 NSApp run loop。Run 直到 Quit 被调用才返回。
// tmux attach / 其他阻塞调用需要放在 Run 之前的子 goroutine 里。
func Run(cfg Config) {
	if !started.CompareAndSwap(false, true) {
		return
	}

	// NSApplication 的激活策略已在 main 中通过 tray.SetupMainApp 设为 Accessory，
	// 这里只需启动 systray。

	systray.Run(func() {
		var icon []byte
		if len(cfg.Icon) > 0 {
			icon = cfg.Icon
		} else {
			icon = loadIcon()
		}
		systray.SetIcon(icon)
		systray.SetTooltip(tooltipText)

		if cfg.Version != "" {
			mVersion := systray.AddMenuItem("fcc v"+cfg.Version, "")
			mVersion.Disable()
			systray.AddSeparator()
		}

		mConfig := systray.AddMenuItem(menuTitleConfig, "")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem(menuTitleExit, "")

		go handleClicks(mConfig.ClickedCh, mQuit.ClickedCh, cfg)
	}, cfg.OnExit)
}

func handleClicks(configCh, quitCh <-chan struct{}, cfg Config) {
	for {
		select {
		case <-configCh:
			if cfg.OnOpenConfig != nil {
				go cfg.OnOpenConfig()
			}
		case <-quitCh:
			// 先杀掉 config 窗口子进程（如果有），再跑用户回调。
			KillConfigWindow()
			if cfg.OnMenuQuit != nil {
				cfg.OnMenuQuit()
			}
			systray.Quit()
			return
		}
	}
}

// Stop 从任意 goroutine 触发菜单栏退出。NSApp 收到 terminate 事件后，
// Run 会返回，调用方可以接着做最终清理。
func Stop() {
	if started.Load() {
		systray.Quit()
	}
}
