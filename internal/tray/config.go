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
	"feishu-connect/internal/updater"
	webview "github.com/webview/webview_go"
)

var (
	configMu   sync.Mutex
	configOpen bool
)

const (
	configWindowTitle     = "fcc — Config"
	firstRunWindowTitle   = "fcc — First Run Setup"
	configWidth           = 520
	configHeight          = 620
	configWindowFlag      = "--config-window"
	firstRunWindowFlag    = "--first-run"
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
func RunConfigWindow(iconPNG []byte, firstRun bool) {
	w := webview.New(false)
	defer w.Destroy()
	if len(iconPNG) > 0 {
		SetAppIcon(iconPNG)
	}
	if firstRun {
		w.SetTitle(firstRunWindowTitle)
	} else {
		w.SetTitle(configWindowTitle)
	}
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

	w.Bind("getUpdateStatus", func() map[string]interface{} {
		state, err := updater.LoadState()
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]interface{}{
			"currentVersion": state.CurrentVersion,
			"latestVersion":  state.LatestVersion,
			"status":         state.Status,
			"hasUpdate":      state.Status == updater.StatusDownloaded && state.Path != "",
		}
	})

	w.Bind("checkUpdate", func() map[string]interface{} {
		state := updater.CheckNow("")
		if state == nil {
			return map[string]interface{}{"success": false, "error": "check failed"}
		}
		if state.Error != "" {
			return map[string]interface{}{"success": false, "error": state.Error}
		}
		return map[string]interface{}{
			"success":        true,
			"currentVersion": state.CurrentVersion,
			"latestVersion":  state.LatestVersion,
			"hasUpdate":      state.Status == updater.StatusDownloaded && state.Path != "",
			"status":         state.Status,
		}
	})

	w.Bind("applyUpdate", func() map[string]interface{} {
		state, err := updater.LoadState()
		if err != nil {
			return map[string]interface{}{"success": false, "error": "load state: " + err.Error()}
		}
		if state.Status != updater.StatusDownloaded || state.Path == "" {
			return map[string]interface{}{"success": false, "error": "no update available"}
		}

		// Replace the binary from the helper process.
		if err := updater.ReplaceBinary(state.Path); err != nil {
			return map[string]interface{}{"success": false, "error": "replace: " + err.Error()}
		}

		// Signal the main process to exit so watchdog restarts the new binary.
		if err := updater.TriggerRestart(); err != nil {
			return map[string]interface{}{"success": false, "error": "restart: " + err.Error()}
		}
		return map[string]interface{}{"success": true}
	})

	w.Bind("saveConfig", func(command string, bypass bool, appID, appSecret string) map[string]interface{} {
		// 读取旧配置，判断 AI 工具配置是否变化（决定是否热重启 tmux）
		oldCfg, _ := config.Load(".env")
		var oldTool string
		var oldBypass bool
		if oldCfg != nil {
			oldTool = detectTool(oldCfg.Command)
			oldBypass = oldCfg.BypassPermissions
		}
		tmuxChanged := oldTool != command || oldBypass != bypass

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

		if firstRun {
			if appID == "" || appSecret == "" {
				return map[string]interface{}{"success": false, "error": "App ID and App Secret are required"}
			}
			// First-run: exit the helper so main process can reload config.
			go w.Terminate()
			return map[string]interface{}{"success": true}
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
    .secret-wrap {
      position: relative;
    }
    .secret-wrap input {
      padding-right: 36px;
    }
    .eye-btn {
      position: absolute;
      right: 6px;
      top: 30%;
      transform: translateY(-50%);
      width: 28px;
      height: 28px;
      padding: 0;
      border: none;
      background: transparent;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
    }
    .eye-btn:hover { opacity: 0.7; background: transparent; outline: none; box-shadow: none; }
    .eye-btn svg {
      width: 16px;
      height: 16px;
      stroke: #666;
      fill: none;
      stroke-width: 2;
      stroke-linecap: round;
      stroke-linejoin: round;
    }
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
    .divider {
      height: 1px;
      background: #ddd;
      margin: 20px 0;
    }
    .update-section {
      margin-top: 4px;
    }
    .update-label {
      font-size: 12px;
      font-weight: 500;
      color: #555;
      margin-bottom: 6px;
    }
    .update-info {
      font-size: 13px;
      color: #333;
    }
    .update-info.uptodate { color: #28a745; }
    .update-info.error { color: #dc3545; }
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
    <div class="secret-wrap">
      <input type="password" id="appSecret" placeholder="xxxxxxxxxxxxxxxxxxxxxx">
      <button type="button" class="eye-btn" id="eyeBtn" onclick="toggleSecret()">
        <svg id="eyeIcon" viewBox="0 0 24 24"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
      </button>
    </div>
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

  <div class="divider"></div>

  <div class="update-section">
    <div class="update-label">Version</div>
    <div id="updateInfo" class="update-info">Checking...</div>
    <div class="update-actions" style="display:flex; gap:8px; margin-top:8px;">
      <button id="checkBtn" onclick="doCheckUpdate()" style="flex:1; margin-top:0;">Check for Updates</button>
      <button id="updateBtn" onclick="doUpdate()" style="display:none; flex:1; margin-top:0;">Restart to Update</button>
    </div>
  </div>

  <script>
    const $ = id => document.getElementById(id);
    const statusEl = $('status');

    function setStatus(text, isError) {
      statusEl.textContent = text;
      statusEl.className = isError ? 'error' : 'success';
    }

    function toggleSecret() {
      const input = $('appSecret');
      const icon = $('eyeIcon');
      if (input.type === 'password') {
        input.type = 'text';
        icon.innerHTML = '<path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/><line x1="2" y1="2" x2="22" y2="22"/>';
      } else {
        input.type = 'password';
        icon.innerHTML = '<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>';
      }
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
      loadUpdateStatus();
    }

    async function loadUpdateStatus() {
      try {
        const s = await window.getUpdateStatus();
        const infoEl = $('updateInfo');
        const btnEl = $('updateBtn');
        if (s.error) {
          infoEl.textContent = 'Update check failed';
          infoEl.className = 'update-info error';
          btnEl.style.display = 'none';
          return;
        }
        if (s.hasUpdate) {
          infoEl.textContent = s.currentVersion + ' → ' + s.latestVersion + ' available';
          infoEl.className = 'update-info';
          btnEl.textContent = 'Restart to Update v' + s.latestVersion;
          btnEl.style.display = '';
        } else if (s.status === 'uptodate') {
          infoEl.textContent = 'v' + s.currentVersion + ' — up to date';
          infoEl.className = 'update-info uptodate';
          btnEl.style.display = 'none';
        } else if (s.status === 'checking' || s.status === 'downloading') {
          infoEl.textContent = 'Checking for updates...';
          infoEl.className = 'update-info';
          btnEl.style.display = 'none';
        } else {
          infoEl.textContent = 'v' + (s.currentVersion || 'unknown');
          infoEl.className = 'update-info';
          btnEl.style.display = 'none';
        }
      } catch (e) {
        $('updateInfo').textContent = 'Update check failed';
        $('updateInfo').className = 'update-info error';
        $('updateBtn').style.display = 'none';
      }
    }

    async function doCheckUpdate() {
      const btn = $('checkBtn');
      const infoEl = $('updateInfo');
      btn.disabled = true;
      btn.textContent = 'Checking...';
      try {
        const res = await window.checkUpdate();
        if (res.success) {
          if (res.hasUpdate) {
            infoEl.textContent = res.currentVersion + ' → ' + res.latestVersion + ' available';
            infoEl.className = 'update-info';
            $('updateBtn').textContent = 'Restart to Update v' + res.latestVersion;
            $('updateBtn').style.display = '';
          } else if (res.status === 'uptodate') {
            infoEl.textContent = 'v' + res.currentVersion + ' — up to date';
            infoEl.className = 'update-info uptodate';
            $('updateBtn').style.display = 'none';
          } else {
            infoEl.textContent = 'v' + (res.currentVersion || 'unknown');
            infoEl.className = 'update-info';
            $('updateBtn').style.display = 'none';
          }
          btn.textContent = 'Check for Updates';
        } else {
          infoEl.textContent = 'Check failed: ' + (res.error || 'unknown');
          infoEl.className = 'update-info error';
          btn.textContent = 'Check for Updates';
        }
      } catch (e) {
        infoEl.textContent = 'Check failed: ' + e.message;
        infoEl.className = 'update-info error';
        btn.textContent = 'Check for Updates';
      }
      btn.disabled = false;
    }

    async function doUpdate() {
      const btn = $('updateBtn');
      btn.disabled = true;
      btn.textContent = 'Updating...';
      try {
        const res = await window.applyUpdate();
        if (res.success) {
          btn.textContent = 'Restarting...';
        } else {
          btn.textContent = 'Update failed: ' + (res.error || 'unknown');
          btn.disabled = false;
        }
      } catch (e) {
        btn.textContent = 'Update failed: ' + e.message;
        btn.disabled = false;
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
