//go:build darwin

package tray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"fcc/internal/log"

	"fcc/internal/config"
	"fcc/internal/updater"
	webview "github.com/webview/webview_go"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

var (
	configMu      sync.Mutex
	configOpen    bool
	configProcess *os.Process
)

const (
	configWindowTitle   = "fcc — Config"
	firstRunWindowTitle = "fcc — First Run Setup"
	configWidth         = 520
	configHeight        = 520
	configWindowFlag    = "--config-window"
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
	if err := cmd.Start(); err != nil {
		return
	}
	configMu.Lock()
	configProcess = cmd.Process
	configMu.Unlock()
	if err := cmd.Wait(); err != nil {
		log.Warnf("[tray] config window exited: %v", err)
	}
}

// KillConfigWindow kills the running config-window helper subprocess, if any.
func KillConfigWindow() {
	configMu.Lock()
	p := configProcess
	configProcess = nil
	configMu.Unlock()
	if p != nil {
		if err := p.Kill(); err != nil {
			log.Debugf("[tray] kill config window: %v", err)
		}
	}
}

// RunConfigWindow is the entry point for the --config-window helper mode.
// It runs the webview in its own main thread and exits when the window closes.
func RunConfigWindow(iconPNG []byte, firstRun bool, version string) {
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
	HideMinimizeAndZoomButtons(w.Window())
	w.SetHtml(configHTML())
	SetupEditMenu()

	w.Bind("loadConfig", func() map[string]interface{} {
		cfg, err := config.Load(".env")
		if err != nil {
			return map[string]interface{}{
				"tool":              "claude",
				"command":           "",
				"bypassPermissions": false,
				"appID":             "",
				"appSecret":         "",
				"error":             err.Error(),
			}
		}
		tool := detectTool(cfg.Command)
		return map[string]interface{}{
			"tool":              tool,
			"command":           cfg.Command,
			"bypassPermissions": cfg.BypassPermissions,
			"appID":             cfg.AppID,
			"appSecret":         cfg.AppSecret,
			"targetName":        cfg.TargetName,
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
		// Run check in background so the webview UI doesn't freeze.
		go updater.CheckNow(version)
		return map[string]interface{}{"checking": true}
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

	w.Bind("saveAiToolConfig", func(tool, command string, bypass bool) map[string]interface{} {
		oldCfg, _ := config.Load(".env")
		var oldCommand string
		var oldBypass bool
		if oldCfg != nil {
			oldCommand = oldCfg.Command
			oldBypass = oldCfg.BypassPermissions
		}

		command, bypass, updates, err := aiToolConfigUpdates(tool, command, bypass)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		if err := config.UpdateEnvVars(".env", updates); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}

		tmuxChanged := oldCommand != command || oldBypass != bypass
		if !firstRun && tmuxChanged {
			signalFCC(syscall.SIGUSR1)
		}
		return map[string]interface{}{"success": true, "tmuxChanged": tmuxChanged}
	})

	w.Bind("saveConfig", func(tool, command string, bypass bool, appID, appSecret, targetName string) map[string]interface{} {
		// Read the old config to decide whether tmux needs to be restarted.
		oldCfg, _ := config.Load(".env")
		var oldCommand string
		var oldBypass bool
		if oldCfg != nil {
			oldCommand = oldCfg.Command
			oldBypass = oldCfg.BypassPermissions
		}
		var updates map[string]string
		var err error
		command, bypass, updates, err = aiToolConfigUpdates(tool, command, bypass)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		if firstRun {
			if appID == "" || appSecret == "" {
				return map[string]interface{}{"success": false, "error": "App ID and App Secret are required"}
			}
		}

		if appID != "" {
			updates["LARK_APP_ID"] = appID
		}
		if appSecret != "" {
			updates["LARK_APP_SECRET"] = appSecret
		}
		if targetName != "" {
			updates["TARGET_NAME"] = targetName
		}
		if err := config.UpdateEnvVars(".env", updates); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}

		tmuxChanged := oldCommand != command || oldBypass != bypass

		if firstRun {
			// First-run: exit the helper so main process can reload config.
			go w.Terminate()
			return map[string]interface{}{"success": true}
		}

		if tmuxChanged {
			signalFCC(syscall.SIGUSR1)
		}

		return map[string]interface{}{"success": true, "tmuxChanged": tmuxChanged}
	})

	w.Bind("updateConfig", func(updates map[string]string) map[string]interface{} {
		if err := config.UpdateEnvVars(".env", updates); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		return map[string]interface{}{"success": true}
	})

	w.Bind("closeWindow", func() {
		go w.Terminate()
	})

	w.Bind("testMessage", func(appID, appSecret, targetName string) map[string]interface{} {
		if appID == "" || appSecret == "" {
			return map[string]interface{}{"success": false, "error": "App ID and App Secret are required"}
		}

		token, err := getTenantAccessToken(appID, appSecret)
		if err != nil {
			return map[string]interface{}{"success": false, "error": "get token: " + err.Error()}
		}

		chatIDs, err := getAllChats(token)
		if err != nil {
			return map[string]interface{}{"success": false, "error": "get chats: " + err.Error()}
		}
		if len(chatIDs) == 0 {
			return map[string]interface{}{"success": false, "error": "no chats found"}
		}

		// Collect all unique members across all chats.
		memberMap := make(map[string]string) // open_id -> name
		for _, chatID := range chatIDs {
			members, err := getChatMembers(token, chatID)
			if err != nil {
				continue
			}
			for _, m := range members {
				memberMap[m.OpenID] = m.Name
			}
		}
		if len(memberMap) == 0 {
			return map[string]interface{}{"success": false, "error": "no members found in any chat"}
		}

		targetName = strings.TrimSpace(targetName)
		if targetName == "" {
			return map[string]interface{}{"success": false, "error": "please enter a Feishu account name to send test message to"}
		}

		targetLower := strings.ToLower(targetName)
		var matchedOpenID, matchedName string
		matchCount := 0
		for openID, name := range memberMap {
			if strings.Contains(strings.ToLower(name), targetLower) {
				matchedOpenID = openID
				matchedName = name
				matchCount++
			}
		}
		if matchCount == 0 {
			return map[string]interface{}{"success": false, "error": "no member named '" + targetName + "' found"}
		}
		if matchCount > 1 {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("found %d members matching '%s', please use exact name", matchCount, targetName)}
		}
		if err := sendTestMessage(token, matchedOpenID); err != nil {
			return map[string]interface{}{"success": false, "error": "send to " + matchedName + ": " + err.Error()}
		}
		return map[string]interface{}{"success": true, "message": "Message sent to " + matchedName}
	})

	w.Run()
}

func aiToolConfigUpdates(tool, command string, bypass bool) (string, bool, map[string]string, error) {
	command = commandFromTool(tool, command)
	if command == "" {
		return "", false, nil, fmt.Errorf("Command is required")
	}
	if tool == "opencode" || tool == "custom" {
		bypass = false
	}
	return command, bypass, map[string]string{
		"COMMAND":            command,
		"BYPASS_PERMISSIONS": boolString(bypass),
	}, nil
}

func signalFCC(sig syscall.Signal) {
	data, err := os.ReadFile("/tmp/fcc.pid")
	if err != nil {
		return
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil {
		if err := syscall.Kill(pid, sig); err != nil {
			log.Debugf("[tray] signal pid %d: %v", pid, err)
		}
	}
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
      padding: 20px 24px 28px;
      color: #333;
      background: #fafafa;
      margin: 0;
    }
    h1 { font-size: 16px; font-weight: 600; margin: 0 0 16px; color: #222; }
    .tabs {
      display: flex;
      gap: 4px;
      margin-bottom: 20px;
      border-bottom: 1px solid #ddd;
    }
    .tab-btn {
      flex: 1;
      padding: 8px 4px;
      border: none;
      background: transparent;
      color: #666;
      font-size: 13px;
      font-weight: 500;
      cursor: pointer;
      border-bottom: 2px solid transparent;
      margin: 0 0 -1px;
      border-radius: 0;
      -webkit-appearance: none;
    }
    .tab-btn:hover {
      background: #f0f0f0;
    }
    .tab-btn.active {
      color: #007aff;
      border-bottom-color: #007aff;
    }
    .tab-content { display: none; }
    .tab-content.active { display: block; }
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
    .secret-wrap { position: relative; }
    .secret-wrap input { padding-right: 36px; }
    .eye-btn {
      position: absolute;
      right: 6px;
      top: 50%;
      transform: translateY(-80%);
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
    button:active { transform: scale(0.98); opacity: 0.9; }
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
    .tab-status {
      margin-top: 12px;
      font-size: 12px;
      min-height: 18px;
    }
    .tab-status.success { color: #28a745; }
    .tab-status.error { color: #dc3545; }
    .hint {
      font-size: 11px;
      color: #999;
      margin-top: 4px;
    }
    .update-section { margin-top: 4px; }
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
    .update-actions {
      display: flex;
      gap: 8px;
      margin-top: 8px;
    }
    .update-actions button {
      flex: 1;
      margin-top: 0;
    }
    .modal-overlay {
      display: none;
      position: fixed;
      top: 0; left: 0; right: 0; bottom: 0;
      background: rgba(0,0,0,0.4);
      align-items: center;
      justify-content: center;
      z-index: 1000;
    }
    .modal-overlay.show { display: flex; }
    .modal-box {
      background: #fff;
      border-radius: 10px;
      padding: 24px 28px;
      min-width: 260px;
      text-align: center;
      box-shadow: 0 8px 32px rgba(0,0,0,0.15);
    }
    .modal-box p {
      margin: 0 0 18px;
      font-size: 15px;
      color: #333;
    }
    .modal-box button {
      margin-top: 0;
      padding: 8px 28px;
      border-radius: 6px;
    }
  </style>
</head>
<body>
  <h1>fcc Configuration</h1>

  <div class="tabs">
    <button class="tab-btn active" data-tab="feishu" onclick="switchTab('feishu')">飞书配置</button>
    <button class="tab-btn" data-tab="ai-tool" onclick="switchTab('ai-tool')">AI Tool配置</button>
    <button class="tab-btn" data-tab="version" onclick="switchTab('version')">版本管理</button>
  </div>

  <div class="tab-content active" id="tab-feishu">
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
    </div>

    <div class="field">
      <label for="targetName">Feishu Account Name</label>
      <input type="text" id="targetName" placeholder="Enter name to send test message to">
      <button id="testBtn" onclick="doTestMessage()" style="background:#34c759; margin-top:16px;">Test Message</button>
      <div id="testStatus" style="margin-top:6px; font-size:12px; min-height:18px;"></div>
    </div>

    <div class="tab-status" id="status-feishu"></div>
  </div>

  <div class="tab-content" id="tab-ai-tool">
    <div class="field">
      <label for="command">AI Tool</label>
      <select id="command">
        <option value="claude">Claude</option>
        <option value="codex">Codex</option>
        <option value="opencode">OpenCode</option>
        <option value="custom">Custom</option>
      </select>
      <div class="hint">The terminal command to bridge via tmux.</div>
    </div>

    <div class="field" id="customCommandField" style="display:none;">
      <label for="customCommand">Command</label>
      <input type="text" id="customCommand" placeholder="bash -il">
    </div>

    <div class="field" id="bypassField">
      <div class="checkbox-wrap">
        <input type="checkbox" id="bypass">
        <label for="bypass">Bypass Permissions</label>
      </div>
      <div class="hint">Skip all permission confirmation prompts (dangerous).</div>
    </div>

    <div class="tab-status" id="status-ai-tool"></div>
  </div>

  <div class="tab-content" id="tab-version">
    <div class="update-section">
      <div class="update-label">Version</div>
      <div id="updateInfo" class="update-info">Checking...</div>
      <div class="update-actions">
        <button id="checkBtn" onclick="doCheckUpdate()">Check for Updates</button>
        <button id="updateBtn" onclick="doUpdate()" style="display:none;">Restart to Update</button>
      </div>
    </div>
  </div>

  <script>
    const $ = id => document.getElementById(id);

    document.addEventListener('keydown', function(e) {
      const key = (e.key || '').toLowerCase();
      if (e.metaKey && key === 'w') {
        e.preventDefault();
        window.closeWindow();
      }
    });

    function showModal(text) {
      $('modalText').textContent = text;
      $('modalOverlay').classList.add('show');
    }
    function hideModal() {
      $('modalOverlay').classList.remove('show');
    }

    function setStatus(text, isError) {
      const activeTab = document.querySelector('.tab-content.active');
      const statusEl = activeTab ? activeTab.querySelector('.tab-status') : null;
      if (statusEl) {
        statusEl.textContent = text;
        statusEl.className = 'tab-status ' + (isError ? 'error' : 'success');
      }
    }

    function switchTab(tab) {
      document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
      document.querySelectorAll('.tab-content').forEach(content => content.classList.remove('active'));
      document.querySelector('.tab-btn[data-tab="' + tab + '"]').classList.add('active');
      $('tab-' + tab).classList.add('active');
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
      const customCommandField = $('customCommandField');
      const isCustom = $('command').value === 'custom';
      customCommandField.style.display = isCustom ? '' : 'none';
      if ($('command').value === 'opencode' || isCustom) {
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
        $('targetName').value = cfg.targetName || '';
        $('command').value = cfg.tool || 'claude';
        $('customCommand').value = cfg.command || '';
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
      infoEl.textContent = 'Checking for updates...';
      infoEl.className = 'update-info';
      $('updateBtn').style.display = 'none';

      try {
        await window.checkUpdate();
        let attempts = 0;
        const poll = setInterval(async () => {
          attempts++;
          if (attempts > 30) {
            clearInterval(poll);
            infoEl.textContent = 'Check timed out. Try again later.';
            infoEl.className = 'update-info error';
            btn.textContent = 'Check for Updates';
            btn.disabled = false;
            return;
          }
          const s = await window.getUpdateStatus();
          if (s.status !== 'checking' && s.status !== 'downloading') {
            clearInterval(poll);
            btn.textContent = 'Check for Updates';
            btn.disabled = false;
            if (s.error) {
              infoEl.textContent = 'Check failed: ' + s.error;
              infoEl.className = 'update-info error';
            } else if (s.hasUpdate) {
              infoEl.textContent = s.currentVersion + ' → ' + s.latestVersion + ' available';
              infoEl.className = 'update-info';
              $('updateBtn').textContent = 'Restart to Update v' + s.latestVersion;
              $('updateBtn').style.display = '';
            } else if (s.status === 'uptodate') {
              infoEl.textContent = 'v' + s.currentVersion + ' — up to date';
              infoEl.className = 'update-info uptodate';
            } else {
              infoEl.textContent = 'v' + (s.currentVersion || 'unknown');
              infoEl.className = 'update-info';
            }
          }
        }, 2000);
      } catch (e) {
        infoEl.textContent = 'Check failed: ' + e.message;
        infoEl.className = 'update-info error';
        btn.textContent = 'Check for Updates';
        btn.disabled = false;
      }
    }

    async function doUpdate() {
      const btn = $('updateBtn');
      btn.disabled = true;
      btn.textContent = 'Updating...';
      try {
        const res = await window.applyUpdate();
        if (res.success) {
          btn.textContent = 'Restarting...';
          window.closeWindow();
        } else {
          btn.textContent = 'Update failed: ' + (res.error || 'unknown');
          btn.disabled = false;
        }
      } catch (e) {
        btn.textContent = 'Update failed: ' + e.message;
        btn.disabled = false;
      }
    }

    async function saveEnv(key, value) {
      setStatus('Saving...');
      try {
        const updates = {[key]: value};
        const res = await window.updateConfig(updates);
        if (res.success) {
          let msg = 'Saved.';
          if (key === 'LARK_APP_ID' || key === 'LARK_APP_SECRET') {
            msg += ' Restart fcc to apply Lark credentials.';
          }
          setStatus(msg, false);
        } else {
          setStatus('Save failed: ' + (res.error || 'unknown'), true);
        }
      } catch (e) {
        setStatus('Save failed: ' + e.message, true);
      }
    }

    async function saveAiTool() {
      setStatus('Saving...');
      try {
        const tool = $('command').value;
        const cmd = tool === 'custom' ? $('customCommand').value.trim() : tool;
        const bypass = (tool === 'opencode' || tool === 'custom') ? false : $('bypass').checked;
        const res = await window.saveAiToolConfig(tool, cmd, bypass);
        if (res.success) {
          let msg = 'Saved.';
          if (res.tmuxChanged) msg += ' Tmux restarted automatically.';
          setStatus(msg, false);
        } else {
          setStatus('Save failed: ' + (res.error || 'unknown'), true);
        }
      } catch (e) {
        setStatus('Save failed: ' + e.message, true);
      }
    }

    async function doTestMessage() {
      const btn = $('testBtn');
      const testStatusEl = $('testStatus');
      btn.disabled = true;
      testStatusEl.textContent = 'Sending...';
      testStatusEl.className = '';
      try {
        const appID = $('appID').value.trim();
        const appSecret = $('appSecret').value.trim();
        if (!appID || !appSecret) {
          testStatusEl.textContent = 'Please enter App ID and App Secret first';
          testStatusEl.className = 'error';
          return;
        }
        const targetName = $('targetName').value.trim();
        const res = await window.testMessage(appID, appSecret, targetName);
        if (res.success) {
          showModal('发送成功');
          testStatusEl.textContent = '';
          testStatusEl.className = '';
        } else {
          testStatusEl.textContent = 'Failed: ' + (res.error || 'unknown');
          testStatusEl.className = 'error';
        }
      } catch (e) {
        testStatusEl.textContent = 'Error: ' + e.message;
        testStatusEl.className = 'error';
      } finally {
        btn.disabled = false;
      }
    }

    $('command').addEventListener('change', () => {
      updateBypassVisibility();
      saveAiTool();
    });
    $('appID').addEventListener('change', () => saveEnv('LARK_APP_ID', $('appID').value.trim()));
    $('appSecret').addEventListener('change', () => saveEnv('LARK_APP_SECRET', $('appSecret').value.trim()));
    $('customCommand').addEventListener('change', saveAiTool);
    $('targetName').addEventListener('change', () => saveEnv('TARGET_NAME', $('targetName').value.trim()));
    $('bypass').addEventListener('change', saveAiTool);
    load();
  </script>

  <div class="modal-overlay" id="modalOverlay">
    <div class="modal-box">
      <p id="modalText"></p>
      <button onclick="hideModal()">确定</button>
    </div>
  </div>
</body>
</html>`
}

// getTenantAccessToken 用 appID + appSecret 获取飞书 tenant_access_token。
func getTenantAccessToken(appID, appSecret string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	})
	resp, err := httpClient.Post("https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("%s", result.Msg)
	}
	return result.TenantAccessToken, nil
}

type memberInfo struct {
	OpenID string
	Name   string
}

// getAllChats 获取 bot 加入的所有群聊的 chat_id。
func getAllChats(token string) ([]string, error) {
	var chatIDs []string
	pageToken := ""
	for {
		url := "https://open.feishu.cn/open-apis/im/v1/chats?page_size=100"
		if pageToken != "" {
			url += "&page_token=" + pageToken
		}
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		var result struct {
			Code int `json:"code"`
			Data struct {
				Items []struct {
					ChatID string `json:"chat_id"`
				} `json:"items"`
				HasMore   bool   `json:"has_more"`
				PageToken string `json:"page_token"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		if result.Code != 0 {
			return nil, fmt.Errorf("code %d", result.Code)
		}
		for _, item := range result.Data.Items {
			chatIDs = append(chatIDs, item.ChatID)
		}
		if !result.Data.HasMore {
			break
		}
		pageToken = result.Data.PageToken
	}
	return chatIDs, nil
}

// getChatMembers 获取指定群聊的所有成员（处理分页）。
func getChatMembers(token, chatID string) ([]memberInfo, error) {
	var members []memberInfo
	pageToken := ""
	for {
		url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/chats/%s/members?page_size=100", chatID)
		if pageToken != "" {
			url += "&page_token=" + pageToken
		}
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		var result struct {
			Code int `json:"code"`
			Data struct {
				Items []struct {
					MemberID string `json:"member_id"`
					Name     string `json:"name"`
				} `json:"items"`
				HasMore   bool   `json:"has_more"`
				PageToken string `json:"page_token"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		if result.Code != 0 {
			return nil, fmt.Errorf("code %d", result.Code)
		}
		for _, item := range result.Data.Items {
			members = append(members, memberInfo{OpenID: item.MemberID, Name: item.Name})
		}
		if !result.Data.HasMore {
			break
		}
		pageToken = result.Data.PageToken
	}
	return members, nil
}

// sendTestMessage 给指定 open_id 发送测试消息。
func sendTestMessage(token, openID string) error {
	body, _ := json.Marshal(map[string]string{
		"receive_id": openID,
		"content":    `{"text":"您好，这是FCC发送的测试消息！"}`,
		"msg_type":   "text",
	})
	req, _ := http.NewRequest("POST", "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("%s", result.Msg)
	}
	return nil
}

// detectTool identifies the selected AI tool from a command string.
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
	return "custom"
}

func commandFromTool(tool, command string) string {
	tool = strings.TrimSpace(tool)
	command = strings.TrimSpace(command)
	switch tool {
	case "claude", "codex", "opencode":
		return tool
	case "custom":
		return command
	default:
		if command != "" {
			return command
		}
		return tool
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
