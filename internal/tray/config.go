//go:build darwin

package tray

import (
	"os"
	"os/exec"
	"sync"

	webview "github.com/webview/webview_go"
)

var (
	configMu   sync.Mutex
	configOpen bool
)

const (
	configWindowTitle = "fcc — Config"
	configWidth       = 480
	configHeight      = 360
	configWindowFlag  = "--config-window"
)

// OpenConfig spawns a helper subprocess to host the config webview. The
// webview library needs the main thread (WKWebView on macOS), but our main
// thread is busy with systray's NSApp run loop — running webview in-process
// would crash the app. The helper exits when the window is closed.
func OpenConfig() {
	configMu.Lock()
	if configOpen {
		configMu.Unlock()
		return
	}
	configOpen = true
	configMu.Unlock()

	defer func() {
		configMu.Lock()
		configOpen = false
		configMu.Unlock()
	}()

	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, configWindowFlag)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()
}

// RunConfigWindow is the entry point for the --config-window helper mode.
// It runs the webview in its own main thread and exits when the window closes.
func RunConfigWindow(iconPNG []byte) {
	w := webview.New(false)
	defer w.Destroy()
	// webview.New 会创建 NSApp 并设 Regular 策略，我们在它之后覆盖图标
	if len(iconPNG) > 0 {
		SetAppIcon(iconPNG)
	}
	w.SetTitle(configWindowTitle)
	w.SetSize(configWidth, configHeight, webview.HintFixed)
	w.SetHtml(placeholderHTML())
	w.Run()
}

func placeholderHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Helvetica Neue", sans-serif;
      padding: 40px 32px;
      text-align: center;
      color: #333;
      background: #fafafa;
    }
    h1 { font-size: 18px; font-weight: 500; margin: 0 0 8px; }
    p  { color: #888; font-size: 13px; margin: 0; }
  </style>
</head>
<body>
  <h1>fcc — Config</h1>
  <p>配置功能开发中。</p>
</body>
</html>`
}
