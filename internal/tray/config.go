//go:build darwin

package tray

import (
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
)

// OpenConfig opens the configuration page in a small native window. If a
// config window is already open, the call is a no-op.
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

	w := webview.New(false)
	defer w.Destroy()
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
