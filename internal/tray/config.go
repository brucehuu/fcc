//go:build darwin

package tray

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"feishu-connect/internal/config"
	webview "github.com/webview/webview_go"
)

var (
	configMu   sync.Mutex
	configOpen bool
)

const (
	configWindowTitle = "fcc — Config"
	configWidth       = 480
	configHeight      = 520
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
	if len(iconPNG) > 0 {
		SetAppIcon(iconPNG)
	}
	w.SetTitle(configWindowTitle)
	w.SetSize(configWidth, configHeight, webview.HintFixed)
	w.SetHtml(configHTML())

	w.Bind("loadConfig", func() map[string]interface{} {
		cfg, err := config.Load(".env")
		if err != nil {
			return map[string]interface{}{
				"command":           "claude",
				"bypassPermissions": false,
				"appID":             "",
				"appSecret":         "",
				"error":             err.Error(),
			}
		}
		return map[string]interface{}{
			"command":           detectTool(cfg.Command),
			"bypassPermissions": cfg.BypassPermissions,
			"appID":             cfg.AppID,
			"appSecret":         cfg.AppSecret,
		}
	})

	w.Bind("saveConfig", func(command string, bypass bool, appID, appSecret string) map[string]interface{} {
		// 读取旧配置，判断 AI 工具配置是否变化（决定是否热重启 tmux）
		oldCfg, _ := config.Load(".env")
		oldTool := detectTool(oldCfg.Command)
		tmuxChanged := oldTool != command || oldCfg.BypassPermissions != bypass

		if err := config.UpdateCommand(".env", command); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		if err := config.UpdateBypassPermissions(".env", bypass); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		if appID != "" {
			if err := config.UpdateAppID(".env", appID); err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}
			}
		}
		if appSecret != "" {
			if err := config.UpdateAppSecret(".env", appSecret); err != nil {
				return map[string]interface{}{"success": false, "error": err.Error()}
			}
		}

		if tmuxChanged {
			data, err := os.ReadFile("/tmp/fcc.pid")
			if err == nil {
				var pid int
				if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil {
					_ = syscall.Kill(pid, syscall.SIGUSR1)
				}
			}
		}

		return map[string]interface{}{"success": true, "tmuxChanged": tmuxChanged}
	})

	w.Run()
}

func configHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    * { box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Helvetica Neue", sans-serif;
      padding: 28px 24px;
      color: #333;
      background: #fafafa;
      margin: 0;
    }
    h1 { font-size: 16px; font-weight: 600; margin: 0 0 20px; color: #222; }
    .field { margin-bottom: 16px; }
    label {
      display: block;
      font-size: 12px;
      font-weight: 500;
      color: #555;
      margin-bottom: 6px;
    }
    select, input[type="text"], input[type="password"], button {
      font-family: inherit;
      font-size: 13px;
      padding: 8px 10px;
      border-radius: 6px;
      border: 1px solid #ccc;
      background: #fff;
      width: 100%;
      outline: none;
    }
    select:focus, input[type="text"]:focus, input[type="password"]:focus { border-color: #007aff; }
    select:focus { border-color: #007aff; }
    button {
      background: #007aff;
      color: #fff;
      border: none;
      font-weight: 500;
      cursor: pointer;
      margin-top: 8px;
    }
    button:hover { background: #0051d5; }
    button:disabled { background: #ccc; cursor: not-allowed; }
    .checkbox-wrap {
      display: flex;
      align-items: center;
      gap: 8px;
      cursor: pointer;
    }
    .checkbox-wrap input[type="checkbox"] {
      width: 16px; height: 16px; margin: 0;
    }
    .checkbox-wrap label {
      margin: 0;
      font-weight: 400;
      cursor: pointer;
    }
    #status {
      margin-top: 12px;
      font-size: 12px;
      min-height: 18px;
    }
    #status.success { color: #28a745; }
    #status.error { color: #dc3545; }
    .hint {
      font-size: 11px;
      color: #999;
      margin-top: 4px;
    }
  </style>
</head>
<body>
  <h1>fcc Configuration</h1>

  <div class="field">
    <label for="appID">Lark App ID</label>
    <input type="text" id="appID" placeholder="cli_xxxxxxxxxxxxx">
  </div>

  <div class="field">
    <label for="appSecret">Lark App Secret</label>
    <input type="password" id="appSecret" placeholder="xxxxxxxxxxxxxxxxxxxxxx">
    <div class="hint">Leave blank to keep current value.</div>
  </div>

  <div class="field">
    <label for="command">AI Tool</label>
    <select id="command">
      <option value="claude">Claude</option>
      <option value="codex">Codex</option>
      <option value="opencode">OpenCode</option>
    </select>
    <div class="hint">The terminal command to bridge via tmux.</div>
  </div>

  <div class="field" id="bypassField">
    <div class="checkbox-wrap">
      <input type="checkbox" id="bypass">
      <label for="bypass">Bypass Permissions</label>
    </div>
    <div class="hint">Skip all permission confirmation prompts (dangerous).</div>
  </div>

  <button id="saveBtn" onclick="doSave()">Save &amp; Apply</button>
  <div id="status"></div>

  <script>
    const $ = id => document.getElementById(id);
    const statusEl = $('status');

    function setStatus(text, isError) {
      statusEl.textContent = text;
      statusEl.className = isError ? 'error' : 'success';
    }

    function updateBypassVisibility() {
      const bypassField = $('bypassField');
      if ($('command').value === 'opencode') {
        bypassField.style.display = 'none';
      } else {
        bypassField.style.display = '';
      }
    }

    async function load() {
      try {
        const cfg = await window.loadConfig();
        if (cfg.error) {
          setStatus('Load failed: ' + cfg.error, true);
          return;
        }
        $('appID').value = cfg.appID || '';
        $('appSecret').value = cfg.appSecret || '';
        $('command').value = cfg.command || 'claude';
        $('bypass').checked = !!cfg.bypassPermissions;
        updateBypassVisibility();
      } catch (e) {
        setStatus('Load failed: ' + e.message, true);
      }
    }

    async function doSave() {
      const btn = $('saveBtn');
      btn.disabled = true;
      setStatus('Saving...');
      try {
        const cmd = $('command').value;
        const bypass = cmd === 'opencode' ? false : $('bypass').checked;
        const appID = $('appID').value.trim();
        const appSecret = $('appSecret').value.trim();
        const res = await window.saveConfig(cmd, bypass, appID, appSecret);
        if (res.success) {
          let msg = 'Saved.';
          if (res.tmuxChanged) msg += ' Tmux restarted automatically.';
          if (appID || appSecret) msg += ' Restart fcc to apply Lark credentials.';
          setStatus(msg, false);
        } else {
          setStatus('Save failed: ' + (res.error || 'unknown'), true);
        }
      } catch (e) {
        setStatus('Save failed: ' + e.message, true);
      }
      btn.disabled = false;
    }

    $('command').addEventListener('change', updateBypassVisibility);
    load();
  </script>
</body>
</html>`
}

// detectTool 从命令字符串中识别 AI 工具名称。
func detectTool(command string) string {
	fields := strings.Fields(command)
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			continue
		}
		base := f
		if idx := strings.LastIndexAny(f, "/\\"); idx >= 0 {
			base = f[idx+1:]
		}
		if dot := strings.LastIndex(base, "."); dot > 0 {
			base = base[:dot]
		}
		switch strings.ToLower(base) {
		case "codex":
			return "codex"
		case "opencode":
			return "opencode"
		case "claude":
			return "claude"
		}
	}
	return "claude"
}
