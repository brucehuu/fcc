//go:build darwin

package tray

import (
	"sync/atomic"

	"github.com/getlantern/systray"
)

const (
	menuTitleConfig = "Open Config"
	menuTitleExit   = "Quit fcc"
	tooltipText     = "fcc — Feishu Connect for Claude"
)

type Config struct {
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
		icon := loadIcon()
		systray.SetIcon(icon)
		systray.SetTooltip(tooltipText)

		mConfig := systray.AddMenuItem(menuTitleConfig, "Open fcc configuration page")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem(menuTitleExit, "Stop fcc and tmux session")

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
			// 先跑「只有菜单 Quit 才该做的事」（比如杀 watchdog），
			// 再触发 NSApp terminate，让 OnExit 跑通用清理（杀 tmux + cancel）。
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
