# feishu-connect

本机终端与飞书的双向实时桥接服务。在本地运行，将终端输出实时推送到飞书，并将飞书私聊/群消息透传给终端进程。

## 使用场景

- **远程开发**：手机飞书与本机 Claude Code / Codex / Aider 等工具持续交互
- **无需公网 IP**：通过飞书 WebSocket 长连接实现双向通信
- **完全透明**：终端开什么工具，飞书就在和什么工具对话，无需任何前缀命令

## 前置依赖

本工具运行前，请确保本机已安装 **tmux**：

```bash
brew install tmux
```

验证安装：

```bash
tmux -V
```

## 快速开始

### 1. 配置文件

复制示例配置并填写你的飞书应用凭证：

```bash
cp env.example env
```

编辑 `env`：

```env
LARK_APP_ID=cli_xxxxxxxxxxxxx
LARK_APP_SECRET=xxxxxxxxxxxxxxxxxxxxxx
COMMAND=claude
```

> ⚠️ **安全提醒**：`env` 文件包含飞书 App Secret 等敏感信息
> - 请确保 `.gitignore` 包含 `env`（项目已默认配置）
> - **不要**把 `env` 文件提交到 git 仓库
> - **不要**通过截图、聊天等方式分享
> - 若怀疑泄露，立即在[飞书开放平台](https://open.feishu.cn/app)重置 Secret

完整配置项说明见 `env.example` 注释。

### 2. 飞书应用配置

1. 登录 [飞书开放平台](https://open.feishu.cn/app)，创建**企业自建应用**
2. 进入应用详情页：
   - **应用能力** → 添加**机器人**
   - **权限管理** → 搜索并开通以下权限：
     - `im:message`
     - `im:message.receive_v1`
     - `im:message:send_as_bot`
   - **事件与回调** → 选择**使用长连接接收事件**模式
3. **创建版本** → 填写版本信息 → **发布**（需要管理员审批）
4. 发布后，在**凭证与基础信息**中获取 `App ID` 和 `App Secret`

### 3. 编译运行

```bash
go build -o feishu-connect .
./feishu-connect
```

或在指定项目目录启动：

```bash
./feishu-connect /Users/yourname/projects/my-project
```

程序会先进入指定目录，再启动配置的命令。启动后自动 attach 到 tmux 会话，本地终端界面正常显示。

**常用操作：**

| 操作 | 说明 |
|------|------|
| `Ctrl+B` 然后按 `D` | 从 tmux detach，程序后台运行，飞书仍保持同步 |
| `Ctrl+C` | 停止整个程序 |
| 重新 attach | `tmux attach -t feishu-connect` |

### 4. 飞书端使用

1. 在飞书搜索你创建的机器人，点击**发消息**进入私聊
2. 直接发送文字，内容会实时透传给本机终端进程
3. 终端进程的输出会实时回传到飞书
4. 群聊中也会接收所有消息（无需 @机器人）

### 5. 中断命令执行

当终端中的工具（如 Claude Code）正在执行耗时操作时，可在飞书端发送以下**精确匹配**的关键词之一来中断当前操作：

| 关键词 | 说明 |
|--------|------|
| `stop` | 英文停止 |
| `esc` | 模拟 ESC 键 |
| `中断` | 中文中断 |
| `取消` | 中文取消 |
| `cancel` | 英文取消 |
| `quit` | 退出 |
| `q` | 简写 |

**注意**：发送时**不要加其他文字**（如"请中断"不会被识别），单独发送一个词即可。中断效果等同于在本地终端按 `ESC` 键。

## 切换开发工具

修改 `env` 中的 `COMMAND`，重启服务即可：

```env
COMMAND=codex      # 使用 Codex
COMMAND=aider      # 使用 Aider
COMMAND=bash       # 使用 Bash
COMMAND=claude --model sonnet   # 带参数
```

### 免确认模式

部分工具支持免确认模式（自动跳过所有权限确认提示）：

```env
BYPASS_PERMISSIONS=true
```

支持：
- `claude` → 自动添加 `--dangerously-skip-permissions`
- `codex` → 自动添加 `--dangerously-bypass-approvals-and-sandbox`

> ⚠️ 此模式会降低安全性，请仅在可信环境下使用。

### Codex 队列模式

```env
CODEX_QUEUE_MODE=guide   # 引导模式（默认）：新消息到达时引导用户进入
CODEX_QUEUE_MODE=queue   # 排队模式：新消息自动排队
```

## 性能调优

通过 `env` 中的可选配置调整行为：

| 配置项 | 默认 | 说明 |
|--------|------|------|
| `CAPTURE_INTERVAL` | `3s` | 终端屏幕捕获间隔，值越小越实时但 CPU 占用越高 |
| `SEND_TIMEOUT` | `10s` | 单条消息发送超时 |
| `SEND_RETRIES` | `3` | 发送失败重试次数 |
| `TMUX_HISTORY_LINES` | `2000` | tmux 历史缓冲区行数 |
| `LOG_LEVEL` | `info` | 日志级别：`debug` / `info` / `warn` / `error` |

## 常见问题

**Q: 启动后终端显示乱码？**

确保系统未设置冲突的 `TERM` 变量。程序启动时会自动将 tmux 终端类型配置为 `screen-256color`。

**Q: 飞书收不到消息？**

检查飞书应用是否已发布，以及机器人是否已添加到私聊会话。

**Q: 飞书发送消息但终端没反应？**

检查 `env` 文件中的 `LARK_APP_ID` 和 `LARK_APP_SECRET` 是否正确，以及应用是否开通了所需权限。

**Q: 指定目录启动后命令执行报错？**

检查路径是否存在、是否有访问权限。路径支持绝对路径和相对路径。

**Q: 想查看完整飞书 ↔ 终端交互日志？**

设置 `LOG_LEVEL=debug` 后所有消息内容（敏感信息已截断）都会输出到 stderr。

## 项目结构

```
feishu-connect/
├── main.go                          # 入口
├── internal/
│   ├── config/config.go             # 配置读取
│   ├── log/log.go                   # 日志包
│   ├── bot/bot.go                   # 飞书 WebSocket + 消息收发
│   ├── terminal/tmux.go             # tmux 封装
│   └── bridge/                      # 桥接逻辑
│       ├── bridge.go                # 核心逻辑
│       ├── interfaces.go            # Messenger/Terminal 抽象
│       └── bridge_test.go           # 单元测试
├── env.example                      # 配置模板
└── README.md                        # 本文档
```

## 依赖

- [larksuite/oapi-sdk-go/v3](https://github.com/larksuite/oapi-sdk-go) - 飞书官方 Go SDK
- [joho/godotenv](https://github.com/joho/godotenv) - .env 文件加载

## 开发

```bash
go test ./...         # 跑单测
go vet ./...          # 静态检查
go build -o feishu-connect .
```
