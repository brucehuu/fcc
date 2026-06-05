package bridge

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIsInterruptCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"stop", true},
		{"STOP", true},
		{"esc", true},
		{"中断", true},
		{"取消", true},
		{"cancel", true},
		{"quit", true},
		{"q", true},
		{"  stop  ", true},    // 前后空格
		{"hello stop", false}, // 含其他字符
		{"please stop", false},
		{"stopping", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isInterruptCommand(tt.input)
		if got != tt.want {
			t.Errorf("isInterruptCommand(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMatchCommand(t *testing.T) {
	tests := []struct {
		command string
		name    string
		want    bool
	}{
		{"claude", "claude", true},
		{"codex", "codex", true},
		{"/usr/local/bin/claude", "claude", true},
		{"./bin/codex", "codex", true},
		{"npx claude", "claude", true},
		{"npx -y codex", "codex", true},
		{"claude --foo", "claude", true},
		{"codex -c desktop.foo=bar", "codex", true},
		{"CLAUDE.exe", "claude", true},
		{"claude.exe --skip", "claude", true},
		{"bash", "claude", false},
		{"aider", "codex", false},
		{"", "claude", false},
		{"-claude", "claude", false},  // 选项被跳过
		{"-- claude", "claude", true}, // "--" 后开始算
	}
	for _, tt := range tests {
		got := matchCommand(tt.command, tt.name)
		if got != tt.want {
			t.Errorf("matchCommand(%q, %q) = %v, want %v", tt.command, tt.name, got, tt.want)
		}
	}
}

func TestIsClaudeAndCodex(t *testing.T) {
	if !isClaudeCommand("claude --dangerously-skip-permissions") {
		t.Error("isClaudeCommand should match claude")
	}
	if isCodexCommand("claude") {
		t.Error("isCodexCommand should not match claude")
	}
	if !isCodexCommand("npx -y codex") {
		t.Error("isCodexCommand should match npx codex")
	}
}

func TestFilterPane(t *testing.T) {
	b := &Bridge{noisePatterns: []string{"fluttering", "nesting", "thinking"}}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"空行过滤",
			"hello\n\nworld\n",
			"hello\nworld",
		},
		{
			"表格边框过滤",
			"┌──┬──┐\n│A│B│\n└──┴──┘",
			"| A | B |\n| --- | --- |",
		},
		{
			"横线过滤",
			"─────\nhello\n─────",
			"hello",
		},
		{
			"ctrl+ 提示过滤",
			"Press Ctrl+C to interrupt\nhello",
			"hello",
		},
		{
			"进度提示过滤",
			"Fluttering about...\nnesting...\nthinking...",
			"",
		},
		{
			"esc 提示过滤",
			"Press esc to interrupt\nfor shortcuts: ctrl+?",
			"",
		},
		{
			"Claude bypass 状态栏过滤",
			"hello\n▸▸ bypass permissions on (shift+tab to cycle)",
			"hello",
		},
		{
			"Claude queued message 提示过滤",
			"hello\nPress up to edit queued messages",
			"hello",
		},
		{
			"TUI 提示符过滤",
			"> \n> hi",
			"> hi",
		},
		{
			"树形符号保留",
			"├── a\n└── b",
			"├── a\n└── b",
		},
		{
			"普通文本保留",
			"hello world\nfoo bar",
			"hello world\nfoo bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.filterPane(tt.input)
			if got != tt.want {
				t.Errorf("filterPane() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestFilterPaneClaudeDynamicProgress(t *testing.T) {
	b := &Bridge{
		isClaude:      true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := "⏺ Bash(grep -rn \"filter\" /Users/huguobiao/code/fcc --include=\"*.py\" | grep -i\n" +
		"○ Explore  Find filter logic for TUI/Tip\n" +
		"lines                         44s\n" +
		"○ Explore  Find filter logic for TUI/Tip lines                         45s\n" +
		"⏺ 你好！有什么我可以帮你的吗？"
	want := "⏺ 你好！有什么我可以帮你的吗？"
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneClaudeToolOutputBlock(t *testing.T) {
	b := &Bridge{
		isClaude:      true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := "⏺ Bash(ls -la)\n" +
		"  ⎿  $ ls -la\n" +
		"     M internal/bridge/bridge.go\n" +
		"     diff --git a/internal/bridge/bridge.go b/internal/bridge/bridge.go\n" +
		"     ok         feishu-connect/internal/bridge  (cached)\n\n" +
		"⏺ 这里是分析结果。"
	want := "⏺ 这里是分析结果。"
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestClaudeDecorativeLineVariants(t *testing.T) {
	tests := []string{
		"✢ Galloping…",
		"\x1b[2K✢ Galloping…",
		"Waiting…",
		"Seasoning... (1.2s)",
		"⎿ \u00a0Tip: Ask Claude to create a todo list",
	}
	for _, input := range tests {
		if !isClaudeDecorativeLine(input) {
			t.Errorf("isClaudeDecorativeLine(%q) = false, want true", input)
		}
	}
}

func TestFilterPaneClaudeUserEcho(t *testing.T) {
	b := &Bridge{
		isClaude:        true,
		noisePatterns:   []string{"fluttering", "nesting", "thinking"},
		lastUserMessage: "老样子，你帮我看看当前的项目有没有什么问题，但是不要做任何动作",
	}
	input := "› 来了老弟\n" +
		"⏺ 好，我先看看项目结构和代码，不做任何改动。"
	want := "⏺ 好，我先看看项目结构和代码，不做任何改动。"
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexNoiseAndToolActivity(t *testing.T) {
	b := &Bridge{
		isCodex:         true,
		noisePatterns:   []string{"fluttering", "nesting", "thinking"},
		lastUserMessage: "先不动手，我们先分析项目情况吧",
	}
	input := strings.Join([]string{
		"› 先不动手，我们先分析项目情况吧",
		"tab to queue message                 96% context left",
		"Tip: Try the Codex App. Run 'codex app' or visit https://chatgpt.com/codex?app-landing-",
		"page=true",
		"• 我先只做只读梳理，不改文件、不启动服务。",
		"• Ran pwd && ls",
		"└ /Users/huguobiao/code/fcc",
		"README.md",
		"... +6 lines (ctrl + t to view transcript)",
		"log",
		"main.go",
		"• Explored",
		"└ List rg --files -g 'README' -g AGENTS.md",
		"-g package.json",
		"• 初步看是一个 Go 项目，分层不大。",
		"─ Worked for 2m 15s ─────────────────────",
		"›",
		"gpt-5.5 high · ~/code/fcc",
	}, "\n")
	want := "• 我先只做只读梳理，不改文件、不启动服务。\n• 初步看是一个 Go 项目，分层不大。"
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexRulesAreOptIn(t *testing.T) {
	b := &Bridge{noisePatterns: []string{"fluttering", "nesting", "thinking"}}
	input := strings.Join([]string{
		"tab to queue message                 96% context left",
		"• Ran pwd && ls",
		"└ /Users/huguobiao/code/fcc",
		"• 我先只做只读梳理。",
	}, "\n")
	if got := b.filterPane(input); got != input {
		t.Errorf("filterPane() =\n%q\nwant unchanged:\n%q", got, input)
	}
}

func TestFilterPaneCodexPlainAlignedTable(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"• 在。给你一个简单表格看效果：",
		"•   项目      数量      状态",
		"───────   ──────   ───────",
		"苹果        3       已购买",
		"───────   ──────   ───────",
		"香蕉        5       待购买",
		"───────   ──────   ───────",
		"橙子        2       已购买",
		"你显示表格是这么",
	}, "\n")
	want := strings.Join([]string{
		"• 在。给你一个简单表格看效果：",
		"| 项目 | 数量 | 状态 |",
		"| --- | --- | --- |",
		"| 苹果 | 3 | 已购买 |",
		"| 香蕉 | 5 | 待购买 |",
		"| 橙子 | 2 | 已购买 |",
		"你显示表格是这么",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexWidePlainTableWithWrappedCells(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"• 可以。当前我这边只看到你给的 AGENTS.md 指令，所以先按这个内容整理成表格：",
		"",
		"   范围                           规则                                    允许使用 seed 数据的情况                                         默认行为",
		"  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		"   /Users/huguobiao/code/fcc      不要用 seed 数据覆盖或刷新项目数据库    1. 首次部署且目标数据库完全为空；2. 你明确要求 reseed/refresh    保留现有项目数据，seed 脚本",
		"   以及本机所有本地项目                                                                                                           应保持非破坏性",
		"",
		"如果你指的是上一轮某个具体结果，把要比较的内容或数据再发我一下，我可以直接按表格重做。",
	}, "\n")
	want := strings.Join([]string{
		"• 可以。当前我这边只看到你给的 AGENTS.md 指令，所以先按这个内容整理成表格：",
		"",
		"| 范围 | 规则 | 允许使用 seed 数据的情况 | 默认行为 |",
		"| --- | --- | --- | --- |",
		"| /Users/huguobiao/code/fcc 以及本机所有本地项目 | 不要用 seed 数据覆盖或刷新项目数据库 | 1. 首次部署且目标数据库完全为空；2. 你明确要求 reseed/refresh | 保留现有项目数据，seed 脚本 应保持非破坏性 |",
		"",
		"如果你指的是上一轮某个具体结果，把要比较的内容或数据再发我一下，我可以直接按表格重做。",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexWideTableKeepsWrappedRowsTogether(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"• 方向                                                     优点                                                 缺点 / 风险                                          适合程度",
		"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━",
		"直接转发终端原始输出                    实现简单；最接近本地终端；不容易漏内容     手机飞书很难读；TUI 边框、提示、工具日志噪    低",
		"                                                                                                                       声多；表格/代码/JSON 容易变形",
		"当前方案：抓 tmux 屏幕 + 过滤 + diff + 飞书卡片  已经能跑通 Codex/Claude；不依赖 Codex 内部  本质靠启发式猜内容类型；容易误判表格/代    中高",
		"                                                    API；能实时更新；兼容多工具                码；TUI 改版会影响过滤规则",
		"",
		"我的判断：短期继续沿用当前方案。",
	}, "\n")
	want := strings.Join([]string{
		"| 方向 | 优点 | 缺点 / 风险 | 适合程度 |",
		"| --- | --- | --- | --- |",
		"| 直接转发终端原始输出 | 实现简单；最接近本地终端；不容易漏内容 | 手机飞书很难读；TUI 边框、提示、工具日志噪 声多；表格/代码/JSON 容易变形 | 低 |",
		"| 当前方案：抓 tmux 屏幕 + 过滤 + diff + 飞书卡片 | 已经能跑通 Codex/Claude；不依赖 Codex 内部 API；能实时更新；兼容多工具 | 本质靠启发式猜内容类型；容易误判表格/代 码；TUI 改版会影响过滤规则 | 中高 |",
		"",
		"我的判断：短期继续沿用当前方案。",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexSuppressesIncompletePlainTable(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"• 可以，下面是一张 Markdown 表格：",
		"",
		"   项目      状态     备注",
		"  ━━━━━━━━  ━━━━━━  ━━━━━━━━━━━━━━━━━━━━",
	}, "\n")
	want := "• 可以，下面是一张 Markdown 表格："
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexStripsBulletFromMarkdownTableHeader(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"• | 维度 | 优点 | 缺点 / 风险 |",
		"| --- | --- | --- |",
		"| 项目定位 | 目标清楚 | 场景强依赖本机环境 |",
	}, "\n")
	want := strings.Join([]string{
		"| 维度 | 优点 | 缺点 / 风险 |",
		"| --- | --- | --- |",
		"| 项目定位 | 目标清楚 | 场景强依赖本机环境 |",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexFiltersExploringToolOutput(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"• 我先只做只读检查。",
		"• Exploring",
		"└ Search AGENTS.md in ..",
		"• 看到这是一个 Go 项目。",
	}, "\n")
	want := strings.Join([]string{
		"• 我先只做只读检查。",
		"• 看到这是一个 Go 项目。",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexFiltersMCPStartupWarnings(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"Tip: Type / to open the command popup; Tab autocompletes slash commands.",
		"⚠ The figma MCP server is not logged in. Run `codex mcp login figma`.",
		"⚠ MCP startup incomplete (failed: figma)",
		"• 我先看一下这个仓库的启动方式和本机端口占用。",
	}, "\n")
	want := "• 我先看一下这个仓库的启动方式和本机端口占用。"
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexFiltersStartupPanelAndLimitWarnings(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"OpenAI Codex (v0.137.0)",
		"model:          gpt-5.5 high   /model to change",
		"directory:      ~/code/fcc",
		"permissions:    YOLO mode",
		"Tip: Try the Codex App. Run 'codex app' or visit https://chatgpt.com/codex?app-landing-page=true",
		"Tip: Use the OpenAI docs MCP for API questions; enable it with codex mcp add openaiDeveloperDocs --url https://developers.openai.com/mcp.",
		"⚠ Heads up, you have less than 25% of your 5h limit left. Run /status for a breakdown.",
		"⚠ The figma MCP server is not logged in. Run `codex mcp login figma`.",
		"⚠ MCP startup incomplete (failed: figma)",
		"• 我先做个轻量环境检查。",
	}, "\n")
	want := "• 我先做个轻量环境检查。"
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexReflowsWrappedTextAfterBulletAnchor(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"• README 和代码显示它是“本机终端 ↔ 飞书”的桥接工具，核心是 tmux 捕获、飞书 WebSocket 收发、菜单栏配置、",
		"飞书表格卡片行高，以及发送 Enter 的方式。",
		"",
		"我看到的几个重点风险：",
		"1. Codex 普通对齐表格识别有误判空间。",
		"   internal/bridge/codex_filter.go:120 目标场景覆盖了，但普",
		"   通“对齐文本”被转成表格或被吞掉的负例还不够。",
		"优点也很明确：表格卡片、markdown 更新、watchdog 都有工程化处理。下一步如果要动，我会先收敛启动副作",
		"用，再补 Codex 表格负例测试。",
	}, "\n")
	want := strings.Join([]string{
		"• README 和代码显示它是“本机终端 ↔ 飞书”的桥接工具，核心是 tmux 捕获、飞书 WebSocket 收发、菜单栏配置、飞书表格卡片行高，以及发送 Enter 的方式。",
		"",
		"我看到的几个重点风险：",
		"1. Codex 普通对齐表格识别有误判空间。",
		"   internal/bridge/codex_filter.go:120 目标场景覆盖了，但普通“对齐文本”被转成表格或被吞掉的负例还不够。",
		"优点也很明确：表格卡片、markdown 更新、watchdog 都有工程化处理。下一步如果要动，我会先收敛启动副作用，再补 Codex 表格负例测试。",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexKeepsParagraphAfterMarkdownList(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"- 表格转飞书 table 卡片: internal/bot/bot.go:435",
		"- 抓 tmux、过滤、diff、发送: internal/bridge/bridge.go:439",
		"- 表格延迟等待完整输出: internal/bridge/bridge.go:579",
		"- Codex 噪声过滤和表格识别: internal/bridge/codex_filter.go:38",
		"",
		"我认为当前最大风险有三个。",
		"",
		"第一，表格识别过度依赖空格对齐。",
	}, "\n")
	want := strings.Join([]string{
		"- 表格转飞书 table 卡片: internal/bot/bot.go:435",
		"- 抓 tmux、过滤、diff、发送: internal/bridge/bridge.go:439",
		"- 表格延迟等待完整输出: internal/bridge/bridge.go:579",
		"- Codex 噪声过滤和表格识别: internal/bridge/codex_filter.go:38",
		"",
		"我认为当前最大风险有三个。",
		"",
		"第一，表格识别过度依赖空格对齐。",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexNormalizesShallowListIndent(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"当前工作区状态有 5 个文件存在本地未提交改动：",
		" - internal/bridge/bridge.go:288: Codex 输入改成 SendLiteral 后延迟 150ms 再发 C-m。",
		"  - internal/bridge/bridge.go:903: 新增/强化 Codex plain table、换行重排、工具输出过滤。",
		"    - 这个是真正的嵌套项。",
		" - internal/bridge/codex_filter.go:1: Codex TUI 过滤规则扩展很多。",
	}, "\n")
	want := strings.Join([]string{
		"当前工作区状态有 5 个文件存在本地未提交改动：",
		"- internal/bridge/bridge.go:288: Codex 输入改成 SendLiteral 后延迟 150ms 再发 C-m。",
		"- internal/bridge/bridge.go:903: 新增/强化 Codex plain table、换行重排、工具输出过滤。",
		"    - 这个是真正的嵌套项。",
		"- internal/bridge/codex_filter.go:1: Codex TUI 过滤规则扩展很多。",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexEmphasizesPlainHeadingLines(t *testing.T) {
	b := &Bridge{
		isCodex:       true,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	input := strings.Join([]string{
		"- internal/bridge/bridge_test.go",
		"- internal/bridge/codex_filter.go",
		"",
		"做得好的地方",
		"核心抽象是合理的。Bridge 通过 Messenger 和 Terminal 抽象隔开飞书和 tmux。",
		"",
		"主要风险",
		"最大风险在启动生命周期。",
	}, "\n")
	want := strings.Join([]string{
		"- internal/bridge/bridge_test.go",
		"- internal/bridge/codex_filter.go",
		"",
		"**做得好的地方**",
		"核心抽象是合理的。Bridge 通过 Messenger 和 Terminal 抽象隔开飞书和 tmux。",
		"",
		"**主要风险**",
		"最大风险在启动生命周期。",
	}, "\n")
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFilterPaneCodexFiltersAnyPromptEcho(t *testing.T) {
	b := &Bridge{
		isCodex:         true,
		noisePatterns:   []string{"fluttering", "nesting", "thinking"},
		lastUserMessage: "帮我简单分析下这个项目呢，只看不做任何动作。",
	}
	input := strings.Join([]string{
		"› 来段Json看看呢",
		"• 我只做只读分析。",
	}, "\n")
	want := "• 我只做只读分析。"
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestClaudeUserEchoLineVariants(t *testing.T) {
	userMsg := "你帮我返回一个表格看一下，我看能不能够正常的展示"
	tests := []string{
		"❯ 你帮我返回一个表格看一下，我看能不能够正常的展示",
		"›\u00a0你帮我返回一个表格看一下，我看能不能够正常的展示",
		"> 你帮我返回一个表格看一下，我看能不能够正常的展示",
	}
	for _, input := range tests {
		if !IsClaudeUserEchoLine(input, userMsg) {
			t.Errorf("IsClaudeUserEchoLine(%q, %q) = false, want true", input, userMsg)
		}
	}

	if IsClaudeUserEchoLine("⏺ 你帮我返回一个表格看一下，我看能不能够正常的展示", userMsg) {
		t.Error("IsClaudeUserEchoLine should not filter normal Claude response lines")
	}

	if !IsClaudeTUIUserPromptLine("› 来了老弟") {
		t.Error("IsClaudeTUIUserPromptLine should match prior Claude prompt echoes")
	}
	if IsClaudeTUIUserPromptLine("> quoted markdown") {
		t.Error("IsClaudeTUIUserPromptLine should not match ASCII markdown blockquotes")
	}
}

func TestDiffPane(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{
			"old 为空时返回 new",
			"",
			"hello\nworld",
			"hello\nworld",
		},
		{
			"无变化返回空",
			"hello",
			"hello",
			"",
		},
		{
			"只返回新增行",
			"a\nb",
			"a\nb\nc",
			"c",
		},
		{
			"顺序无关",
			"a\nb",
			"c\nb",
			"c",
		},
		{
			"空行忽略",
			"a\n\nb",
			"a\nc",
			"c",
		},
		{
			"重复行保留",
			"a\nb",
			"a\nb\na",
			"a",
		},
		{
			"old 中重复行",
			"a\na",
			"a\na\na",
			"a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffPane(tt.old, tt.new)
			if got != tt.want {
				t.Errorf("diffPane() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertTableLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"基本两列",
			"│ A │ B │",
			"| A | B |",
		},
		{
			"三列",
			"│ 1 │ 2 │ 3 │",
			"| 1 | 2 | 3 |",
		},
		{
			"保留空 cell",
			"│ A │  │ B │",
			"| A |  | B |",
		},
		{
			"全空保留空列",
			"│ │ │",
			"|  |  |",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertTableLine(tt.input)
			if got != tt.want {
				t.Errorf("convertTableLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsMarkdownTableLine(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"| A | B |", true},
		{"| A|B |", true},
		{"  | A | B |  ", true},
		{"|A|", true},
		{"hello", false},
		{"| no end", false},
		{"no start |", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isMarkdownTableLine(tt.input)
		if got != tt.want {
			t.Errorf("isMarkdownTableLine(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSplitDiffIntoBlocks(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n| 1 | 2 |\nplain text\nanother line"
	got := splitDiffIntoBlocks(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %v", len(got), got)
	}
	if !isMarkdownTable(got[0]) {
		t.Errorf("first block should be table: %q", got[0])
	}
	if isMarkdownTable(got[1]) {
		t.Errorf("second block should be text: %q", got[1])
	}
}

func TestMatchCommandDebounceLogic(t *testing.T) {
	// 验证去抖逻辑：两次中断触发间隔 < 阈值时只算一次
	// 通过 Bridge.lastInterrupt 字段模拟
	b := &Bridge{interruptDebounce: 500 * time.Millisecond}
	b.lastInterrupt = time.Now()

	// 第一次触发：被去抖
	if time.Since(b.lastInterrupt) >= b.interruptDebounce {
		t.Error("first call should be debounced")
	}
	// 等待超过阈值
	time.Sleep(600 * time.Millisecond)
	if time.Since(b.lastInterrupt) < b.interruptDebounce {
		t.Error("after sleep, should not be debounced")
	}
}

func TestSendUserInputToTerminalCodexUsesRealEnter(t *testing.T) {
	tm := &mockTerminal{}
	b := &Bridge{isCodex: true}

	if err := b.sendUserInputToTerminal(tm, "给我一个表格"); err != nil {
		t.Fatalf("sendUserInputToTerminal() error = %v", err)
	}

	if len(tm.sentKeys) != 0 {
		t.Fatalf("Codex should not use SendKeys, got %v", tm.sentKeys)
	}
	if len(tm.sentLiteral) != 1 || tm.sentLiteral[0] != "给我一个表格" {
		t.Fatalf("sentLiteral = %v, want [给我一个表格]", tm.sentLiteral)
	}
	if len(tm.sentSpecial) != 1 || tm.sentSpecial[0] != "C-m" {
		t.Fatalf("sentSpecial = %v, want [C-m]", tm.sentSpecial)
	}
}

func TestSendUserInputToTerminalNonCodexKeepsLegacySendKeys(t *testing.T) {
	tm := &mockTerminal{}
	b := &Bridge{}

	if err := b.sendUserInputToTerminal(tm, "hello"); err != nil {
		t.Fatalf("sendUserInputToTerminal() error = %v", err)
	}

	if len(tm.sentKeys) != 1 || tm.sentKeys[0] != "hello" {
		t.Fatalf("sentKeys = %v, want [hello]", tm.sentKeys)
	}
	if len(tm.sentLiteral) != 0 || len(tm.sentSpecial) != 0 {
		t.Fatalf("non-Codex should not use literal/special, got literal=%v special=%v", tm.sentLiteral, tm.sentSpecial)
	}
}

func TestSplitDiffIntoBlocksEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		isTable0 bool
	}{
		{"empty", "", 0, false},
		{"all text", "hello\nworld", 1, false},
		{"all table", "| A | B |\n| --- | --- |\n| 1 | 2 |", 1, true},
		{"table rows without separator", "| 1 | 2 |\n| 3 | 4 |", 1, false},
		{"mixed", "| A | B |\nplain", 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitDiffIntoBlocks(tt.input)
			if len(got) != tt.wantLen {
				t.Fatalf("expected %d blocks, got %d: %v", tt.wantLen, len(got), got)
			}
			if len(got) > 0 && isMarkdownTable(got[0]) != tt.isTable0 {
				t.Errorf("first block isTable = %v, want %v", isMarkdownTable(got[0]), tt.isTable0)
			}
		})
	}
}

func TestSplitDiffIntoBlocksReflowsWrappedMarkdownTableRows(t *testing.T) {
	input := strings.Join([]string{
		"intro",
		"| 维度 | 优点 | 缺点 |",
		"| --- | --- | --- |",
		"| 架构设计 | 模块划分清晰（bot/bridge/terminal/tray/updater/watchdog/config/log），职责边界明确；通过",
		"Messenger / Terminal",
		"接口抽象实现飞书与终端的解耦 | bridge.go 单文件 1237 行，承载了",
		"diff、过滤、格式化、发送、累积等过多职责 |",
		"outro",
	}, "\n")

	got := splitDiffIntoBlocks(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %#v", len(got), got)
	}
	if got[0] != "intro" || got[2] != "outro" {
		t.Fatalf("text blocks = %#v, want intro/table/outro", got)
	}
	if !isMarkdownTable(got[1]) {
		t.Fatalf("middle block should be markdown table:\n%s", got[1])
	}
	lines := strings.Split(got[1], "\n")
	if len(lines) != 3 {
		t.Fatalf("table lines = %d, want 3: %#v", len(lines), lines)
	}
	if cells := parseTableCells(lines[2]); len(cells) != 3 {
		t.Fatalf("row cells = %#v, want 3 cells", cells)
	}
}

func TestFlushTableEdgeCases(t *testing.T) {
	// single column table: should add separator row
	got := flushTable([]string{"| A |"})
	if len(got) != 2 || got[0] != "| A |" || got[1] != "| --- |" {
		t.Errorf("flushTable single column = %v, want [| A | | --- |]", got)
	}

	// zero column table
	got = flushTable([]string{"| |"})
	if len(got) != 2 || got[0] != "| |" || got[1] != "| --- |" {
		t.Errorf("flushTable zero column = %v, want [| | | --- |]", got)
	}

	// normal table
	got = flushTable([]string{"| A | B |", "| 1 | 2 |"})
	if len(got) != 3 {
		t.Fatalf("flushTable normal = %v, want 3 lines", got)
	}
	if got[1] != "| --- | --- |" {
		t.Errorf("flushTable separator = %q, want | --- | --- |", got[1])
	}
}

func TestCompactMarkdownSpacing(t *testing.T) {
	input := "\nhello  \n\n\nworld\n\n\n- a\n\n- b\n\n"
	want := "hello\n\nworld\n\n- a\n\n- b"
	if got := compactMarkdownSpacing(input); got != want {
		t.Errorf("compactMarkdownSpacing() = %q, want %q", got, want)
	}
}

func TestFormatBlockToMarkdownFencesJSONLikeBlock(t *testing.T) {
	input := strings.Join([]string{
		"输出：飞书 Interactive 卡片 JSON",
		"{",
		`"config": {`,
		`"wide_screen_mode": true`,
		"},",
		`"elements": [`,
		"]",
		"}",
	}, "\n")

	got := formatBlockToMarkdown(input)
	if !strings.Contains(got, "```json\n{") {
		t.Fatalf("expected fenced json block, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n```") {
		t.Fatalf("expected closing code fence, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "输出：飞书 Interactive 卡片 JSON\n\n") {
		t.Fatalf("expected prefix to be preserved, got:\n%s", got)
	}
}

func TestFormatBlockToMarkdownKeepsExistingFence(t *testing.T) {
	input := "```json\n{}\n```"
	if got := formatBlockToMarkdown(input); got != input {
		t.Fatalf("formatBlockToMarkdown() = %q, want %q", got, input)
	}
}

func TestFormatBlockToMarkdownDropsStaleCodePrefixBeforeCodexBullet(t *testing.T) {
	input := strings.Join([]string{
		"const { isFirstDeploy = false, isDatabaseEmpty = false, explicit",
		"• 我先只做只读检查：看目录结构、Git 状态、关键配置和入口文件，不改文件、不启动服务。",
		"• 看到这是一个 Go 项目，当前工作树已经有未提交修改。",
	}, "\n")

	got := formatBlockToMarkdown(input)
	if strings.Contains(got, "```") {
		t.Fatalf("did not expect code fence, got:\n%s", got)
	}
	if strings.Contains(got, "isFirstDeploy") {
		t.Fatalf("expected stale code prefix to be dropped, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "• 我先只做只读检查：") {
		t.Fatalf("expected codex bullet prose to be preserved, got:\n%s", got)
	}
}

func TestFilterPanePreservesCodeIndentation(t *testing.T) {
	b := &Bridge{isCodex: true}
	input := strings.Join([]string{
		"• JSON 示例：",
		"{",
		`  "project": "fcc",`,
		`  "tasks": [`,
		"    {",
		`      "id": 1`,
		"    }",
		"  ]",
		"}",
	}, "\n")
	want := input
	if got := b.filterPane(input); got != want {
		t.Errorf("filterPane() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFormatBlockToMarkdownFencesJavaScriptCodeBlock(t *testing.T) {
	input := strings.Join([]string{
		"• function formatStatus(task) {",
		"  const statusMap = {",
		`    pending: "未开始",`,
		`    active: "进行中",`,
		`    done: "已完成",`,
		"  };",
		"  return `${task.name}: ${statusMap[task.status] ?? \"未知状态\"}`;",
		"}",
		`console.log(formatStatus({ name: "代码检查", status: "active" }));`,
	}, "\n")

	got := formatBlockToMarkdown(input)
	if !strings.Contains(got, "```javascript\nfunction formatStatus(task) {") {
		t.Fatalf("expected fenced javascript block, got:\n%s", got)
	}
	if !strings.Contains(got, "  const statusMap = {") || !strings.Contains(got, `    pending: "未开始",`) {
		t.Fatalf("expected indentation to be preserved, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n```") {
		t.Fatalf("expected closing code fence, got:\n%s", got)
	}
}

func TestFormatBlockToMarkdownFencesGoCodeBlock(t *testing.T) {
	input := strings.Join([]string{
		"这是一个 Go 语言 HTTP 服务器的代码片段：",
		"package main",
		"import (",
		`"encoding/json"`,
		`"net/http"`,
		")",
		"type Response struct {",
		"Message string `json:\"message\"`",
		"}",
		"func main() {",
		`http.HandleFunc("/health", nil)`,
		"}",
		"如果你有特定的编程语言或功能需求，告诉我。",
	}, "\n")

	got := formatBlockToMarkdown(input)
	if !strings.Contains(got, "```go\npackage main") {
		t.Fatalf("expected fenced go block, got:\n%s", got)
	}
	if !strings.Contains(got, "\n```\n\n如果你有特定的编程语言或功能需求，告诉我。") {
		t.Fatalf("expected prose suffix outside fence, got:\n%s", got)
	}
}

func TestSendMarkdownBlocksBuffersStreamingJSONUntilClosed(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:   ms,
		sendTimeout: 10 * time.Second,
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{}
	b.receivers.Store(key, state)

	ctx := context.Background()
	b.sendMarkdownBlocks(ctx, key, state, []string{strings.Join([]string{
		"显示一段Json",
		"🔘 {",
		`"project": {`,
		`"name": "api-gateway"`,
		"},",
		`"servers": [`,
	}, "\n")})
	if state.pendingCodeLang != "json" {
		t.Fatalf("expected pending json, got lang=%q", state.pendingCodeLang)
	}

	b.sendMarkdownBlocks(ctx, key, state, []string{strings.Join([]string{
		"{",
		`"id": "srv-001",`,
		`"host": "10.0.1.15"`,
		"},",
		"{",
		`"id": "srv-002",`,
		`"host": "10.0.1.16"`,
		"}",
		"],",
		`"status": "active"`,
		"}",
	}, "\n")})

	texts := ms.Texts()
	if len(texts) == 0 {
		t.Fatal("expected markdown text after json closes")
	}
	got := texts[len(texts)-1]
	if strings.Count(got, "```json") != 1 || !strings.HasSuffix(got, "\n```") {
		t.Fatalf("expected one json fence, got:\n%s", got)
	}
	if strings.Contains(got, "🔘") {
		t.Fatalf("expected status marker removed from code, got:\n%s", got)
	}
}

func TestSendMarkdownBlocksBuffersStreamingGoUntilFunctionCloses(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:   ms,
		sendTimeout: 10 * time.Second,
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{}
	b.receivers.Store(key, state)

	ctx := context.Background()
	b.sendMarkdownBlocks(ctx, key, state, []string{strings.Join([]string{
		"worker 从 jobs 通道接收任务，处理后将结果发送到 results 通道",
		"func worker(id int, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {",
		"defer wg.Done()",
		"for job := range jobs {",
		`fmt.Printf("Worker %d 正在处理 Job %d\n", id, job.ID)`,
	}, "\n")})
	if state.pendingCodeLang != "go" {
		t.Fatalf("expected pending go, got lang=%q", state.pendingCodeLang)
	}
	if len(ms.Texts()) == 0 || strings.Contains(ms.Texts()[len(ms.Texts())-1], "```go") {
		t.Fatalf("first chunk should not send incomplete go fence, texts=%v", ms.Texts())
	}

	b.sendMarkdownBlocks(ctx, key, state, []string{strings.Join([]string{
		"// 模拟耗时操作：计算 1 到 Value 的和",
		"sum := 0",
		"for i := 1; i <= job.Value; i++ {",
		"sum += i",
		"time.Sleep(10 * time.Millisecond) // 模拟工作负载",
		"}",
		"results <- Result{JobID: job.ID, Sum: sum}",
		"}",
		"}",
		"后续说明文字",
	}, "\n")})

	texts := ms.Texts()
	if len(texts) == 0 {
		t.Fatal("expected markdown text after go function closes")
	}
	got := texts[len(texts)-1]
	if strings.Count(got, "```go") != 1 || !strings.Contains(got, "// 模拟耗时操作") {
		t.Fatalf("expected one complete go fence with comments, got:\n%s", got)
	}
	if !strings.Contains(got, "\n```\n后续说明文字") {
		t.Fatalf("expected suffix outside go fence, got:\n%s", got)
	}
}

func TestSendMarkdownBlocksBuffersStreamingJavaScriptUntilClosed(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:   ms,
		sendTimeout: 10 * time.Second,
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{}
	b.receivers.Store(key, state)

	ctx := context.Background()
	b.sendMarkdownBlocks(ctx, key, state, []string{strings.Join([]string{
		"• 可以，随便来一段实用点的：",
		"• function debounce(fn, delay = 300) {",
		"  let timer = null;",
		"  return function (...args) {",
		"    clearTimeout(timer);",
		"    timer = setTimeout(() => {",
		"      fn.apply(this, args);",
		"    }, delay);",
		"  };",
		"}",
		"// 示例",
		"const onSearch = debounce((keyword) => {",
	}, "\n")})
	if state.pendingCodeLang != "javascript" {
		t.Fatalf("expected pending javascript, got lang=%q", state.pendingCodeLang)
	}
	texts := ms.Texts()
	if len(texts) == 0 || strings.Contains(texts[len(texts)-1], "```javascript") {
		t.Fatalf("first chunk should not send incomplete javascript fence, texts=%v", texts)
	}

	b.sendMarkdownBlocks(ctx, key, state, []string{strings.Join([]string{
		`  console.log("search:", keyword);`,
		"}, 500);",
		`onSearch("a");`,
		`onSearch("ab");`,
		`onSearch("abc"); // 只有最后一次会在 500ms 后执行`,
		"这个小片段挺常见。",
	}, "\n")})

	texts = ms.Texts()
	if len(texts) == 0 {
		t.Fatal("expected markdown text after javascript closes")
	}
	got := texts[len(texts)-1]
	if strings.Count(got, "```javascript") != 1 {
		t.Fatalf("expected one javascript fence, got:\n%s", got)
	}
	if !strings.Contains(got, `console.log("search:", keyword);`) ||
		!strings.Contains(got, `onSearch("abc"); // 只有最后一次会在 500ms 后执行`) {
		t.Fatalf("expected full javascript code inside fence, got:\n%s", got)
	}
	if !strings.Contains(got, "\n```\n这个小片段挺常见。") {
		t.Fatalf("expected prose suffix outside fence, got:\n%s", got)
	}
}

func TestFlushPendingCodeIfReady(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:   ms,
		sendTimeout: 10 * time.Second,
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{
		pendingCodeLang:  "go",
		pendingCode:      []string{"func main() {", `fmt.Println("hi")`},
		pendingCodeSince: time.Now().Add(-6 * time.Second),
	}
	b.receivers.Store(key, state)

	if !b.flushPendingCodeIfReady(context.Background(), key, state, 5*time.Second) {
		t.Fatal("expected pending code to flush")
	}
	texts := ms.Texts()
	if len(texts) != 1 || !strings.Contains(texts[0], "```go\nfunc main()") {
		t.Fatalf("texts = %v", texts)
	}
	if len(state.pendingCode) != 0 || state.pendingCodeLang != "" {
		t.Fatalf("pending code not cleared: %#v", state)
	}
}

func TestSendMarkdownBlocksUsesTightSpacing(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:   ms,
		sendTimeout: 10 * time.Second,
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{}
	b.receivers.Store(key, state)

	ctx := context.Background()
	b.sendMarkdownBlocks(ctx, key, state, []string{"hello", "world"})
	b.sendMarkdownBlocks(ctx, key, state, []string{"foo\n\n\nbar"})

	texts := ms.Texts()
	if len(texts) != 2 {
		t.Fatalf("expected send + update, got %d messages: %v", len(texts), texts)
	}
	want := "hello\nworld\nfoo\n\nbar"
	if texts[1] != want {
		t.Errorf("updated markdown = %q, want %q", texts[1], want)
	}
}

func TestSendMarkdownContentOpensNewCardWhenFull(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:   ms,
		sendTimeout: 10 * time.Second,
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{}
	b.receivers.Store(key, state)

	ctx := context.Background()
	nearLimit := strings.Repeat("a", maxMarkdownLen-10)
	b.sendMarkdownContent(ctx, key, state, nearLimit)
	b.sendMarkdownContent(ctx, key, state, "second card content")

	texts := ms.Texts()
	if len(texts) != 2 {
		t.Fatalf("expected first card send and second card send, got %d messages", len(texts))
	}
	if strings.Contains(texts[1], "前面内容已省略") {
		t.Fatalf("second card should not use truncation marker: %q", texts[1])
	}
	if texts[1] != "second card content" {
		t.Errorf("second card = %q, want new content only", texts[1])
	}
}

func TestSplitMarkdownContent(t *testing.T) {
	got := splitMarkdownContent("aaa\nbbb\nccc", 7)
	want := []string{"aaa", "bbb\nccc"}
	if len(got) != len(want) {
		t.Fatalf("splitMarkdownContent len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitMarkdownContent[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsHorizontalBorderEdgeCases(t *testing.T) {
	if isHorizontalBorder("") {
		t.Error("isHorizontalBorder(\"\") should be false")
	}
	if isHorizontalBorder("──") {
		t.Error("isHorizontalBorder short should be false")
	}
	if !isHorizontalBorder("─────") {
		t.Error("isHorizontalBorder(\"─────\") should be true")
	}
}

// mockMessenger 用于测试
type mockMessenger struct {
	texts  []string
	tables []string
	mu     sync.Mutex
}

func (m *mockMessenger) Start(ctx context.Context) error { return nil }
func (m *mockMessenger) SendText(ctx context.Context, _, _, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	return nil
}
func (m *mockMessenger) SendInteractiveTable(ctx context.Context, _, _, table string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tables = append(m.tables, table)
	return nil
}
func (m *mockMessenger) SendMarkdown(ctx context.Context, _, _, text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	return "mock-msg-id", nil
}
func (m *mockMessenger) UpdateMessage(ctx context.Context, messageID, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, content)
	return nil
}
func (m *mockMessenger) SendWelcome(ctx context.Context, targetName, text string) error { return nil }
func (m *mockMessenger) CleanupOldImages(maxAge time.Duration) error                    { return nil }
func (m *mockMessenger) Close()                                                         {}

func (m *mockMessenger) Texts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.texts...)
}

func (m *mockMessenger) Tables() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.tables...)
}

// mockTerminal 用于测试
type mockTerminal struct {
	captures    []string
	index       int
	sentKeys    []string
	sentLiteral []string
	sentSpecial []string
	mu          sync.Mutex
}

func (t *mockTerminal) Start(command, workDir string) error { return nil }
func (t *mockTerminal) SendKeys(text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sentKeys = append(t.sentKeys, text)
	return nil
}
func (t *mockTerminal) SendLiteral(text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sentLiteral = append(t.sentLiteral, text)
	return nil
}
func (t *mockTerminal) SendSpecialKey(key string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sentSpecial = append(t.sentSpecial, key)
	return nil
}
func (t *mockTerminal) CaptureVisible(historyLines int) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.index >= len(t.captures) {
		return "", nil
	}
	s := t.captures[t.index]
	t.index++
	return s, nil
}
func (t *mockTerminal) WaitReady() error  { return nil }
func (t *mockTerminal) Kill() error       { return nil }
func (t *mockTerminal) HasSession() bool  { return true }
func (t *mockTerminal) IsAvailable() bool { return true }

func TestCaptureAndSend(t *testing.T) {
	tm := &mockTerminal{
		captures: []string{
			"hello\nworld",      // tick 1: baseline
			"hello\nworld\nfoo", // tick 2: diff = foo
			"hello\nworld\nfoo", // tick 3: no diff
		},
	}
	ms := &mockMessenger{}

	b := &Bridge{
		messenger:       ms,
		term:            tm,
		captureInterval: 3 * time.Second,
		sendTimeout:     10 * time.Second,
		historyLines:    2000,
		noisePatterns:   []string{"fluttering", "nesting", "thinking"},
	}

	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{lastPane: "hello\nworld", ready: true}
	b.receivers.Store(key, state)

	ctx := context.Background()

	// tick 1: no diff (same as baseline)
	b.captureAndSend(ctx)
	time.Sleep(50 * time.Millisecond)
	if len(ms.Texts()) != 0 {
		t.Errorf("tick 1: expected 0 messages, got %d", len(ms.Texts()))
	}

	// tick 2: diff = "foo", 文本内容直接发送
	b.captureAndSend(ctx)
	time.Sleep(50 * time.Millisecond)
	texts := ms.Texts()
	if len(texts) != 1 {
		t.Errorf("tick 2: expected 1 message, got %d", len(texts))
	} else if texts[0] != "foo" {
		t.Errorf("tick 2: expected 'foo', got %q", texts[0])
	}

	// tick 3: no diff
	b.captureAndSend(ctx)
	time.Sleep(50 * time.Millisecond)
	if len(ms.Texts()) != 1 {
		t.Errorf("tick 3: expected 1 message total, got %d", len(ms.Texts()))
	}
}

func TestCaptureAndSendTable(t *testing.T) {
	tm := &mockTerminal{
		captures: []string{
			"header\n│ A │ B │\n│ 1 │ 2 │\n│ 3 │ 4 │",
		},
	}
	ms := &mockMessenger{}

	b := &Bridge{
		messenger:       ms,
		term:            tm,
		captureInterval: 3 * time.Second,
		sendTimeout:     10 * time.Second,
		historyLines:    2000,
		noisePatterns:   []string{"fluttering", "nesting", "thinking"},
	}

	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{ready: true}
	b.receivers.Store(key, state)

	ctx := context.Background()
	b.captureAndSend(ctx)
	time.Sleep(50 * time.Millisecond)

	if len(ms.Tables()) != 0 {
		t.Fatalf("table should be buffered before idle flush, got %v", ms.Tables())
	}
	texts := ms.Texts()
	if len(texts) != 1 {
		t.Errorf("expected 1 markdown text message, got %d", len(texts))
	} else if texts[0] != "header" {
		t.Errorf("expected text header, got %q", texts[0])
	}

	state.pendingTableSince = time.Now().Add(-6 * time.Second)
	if !b.flushPendingTableIfReady(ctx, key, state, 5*time.Second) {
		t.Fatal("expected idle flush to send buffered table")
	}
	tables := ms.Tables()
	if len(tables) != 1 {
		t.Errorf("expected 1 table message after idle flush, got %d", len(tables))
	}
}

func TestSendBlocksBuffersStreamingTableRows(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:     ms,
		sendTimeout:   10 * time.Second,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{}
	b.receivers.Store(key, state)

	ctx := context.Background()
	b.sendBlocks(ctx, key, "intro\n| 功能 | 状态 |\n| --- | --- |\n| 用户认证 | 已完成 |")

	if len(ms.Tables()) != 0 {
		t.Fatalf("first partial table should be buffered, got tables: %v", ms.Tables())
	}

	b.sendBlocks(ctx, key, "| 数据导出 | 进行中 |\n| 移动端适配 | 已完成 |\n请确认")

	tables := ms.Tables()
	if len(tables) != 1 {
		t.Fatalf("expected 1 merged table, got %d: %v", len(tables), tables)
	}
	wantTable := "| 功能 | 状态 |\n| --- | --- |\n| 用户认证 | 已完成 |\n| 数据导出 | 进行中 |\n| 移动端适配 | 已完成 |"
	if tables[0] != wantTable {
		t.Errorf("merged table =\n%q\nwant:\n%q", tables[0], wantTable)
	}

	texts := ms.Texts()
	if len(texts) != 2 || texts[0] != "intro" || texts[1] != "请确认" {
		t.Errorf("texts = %v, want [intro 请确认]", texts)
	}
}

func TestSendBlocksBuffersSingleMarkdownTableHeader(t *testing.T) {
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:     ms,
		sendTimeout:   10 * time.Second,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{}
	b.receivers.Store(key, state)

	ctx := context.Background()
	b.sendBlocks(ctx, key, "我对后续优先级的判断大概是：\n| 优先级 | 方向 | 原因 |")

	if len(ms.Texts()) != 1 || ms.Texts()[0] != "我对后续优先级的判断大概是：" {
		t.Fatalf("texts after header = %v, want intro only", ms.Texts())
	}
	if len(ms.Tables()) != 0 {
		t.Fatalf("table header should be buffered, got tables: %v", ms.Tables())
	}
	if strings.Join(state.pendingTable, "\n") != "| 优先级 | 方向 | 原因 |" {
		t.Fatalf("pendingTable = %v", state.pendingTable)
	}

	state.pendingTableSince = time.Now().Add(-6 * time.Second)
	if b.flushPendingTableIfReady(ctx, key, state, 5*time.Second) {
		t.Fatal("incomplete table header should not flush on idle")
	}

	b.sendBlocks(ctx, key, "| --- | --- | --- |\n| P0 | 收敛启动 | 最容易造成诡异故障 |")

	if len(ms.Tables()) != 0 {
		t.Fatalf("completed table should wait for idle flush, got %v", ms.Tables())
	}
	state.pendingTableSince = time.Now().Add(-6 * time.Second)
	if !b.flushPendingTableIfReady(ctx, key, state, 5*time.Second) {
		t.Fatal("expected completed table to flush on idle")
	}

	tables := ms.Tables()
	if len(tables) != 1 {
		t.Fatalf("expected one completed table, got %d: %v", len(tables), tables)
	}
	wantTable := "| 优先级 | 方向 | 原因 |\n| --- | --- | --- |\n| P0 | 收敛启动 | 最容易造成诡异故障 |"
	if tables[0] != wantTable {
		t.Fatalf("table =\n%q\nwant:\n%q", tables[0], wantTable)
	}
}

func TestCaptureAndSendBuffersCodexBulletMarkdownTableHeader(t *testing.T) {
	tm := &mockTerminal{
		captures: []string{
			"base\n• | 维度 | 优点 | 缺点 / 风险 |",
		},
	}
	ms := &mockMessenger{}
	b := &Bridge{
		messenger:     ms,
		term:          tm,
		sendTimeout:   10 * time.Second,
		historyLines:  2000,
		noisePatterns: []string{"fluttering", "nesting", "thinking"},
		isCodex:       true,
	}
	key := receiverKey{id: "user1", kind: "open_id"}
	state := &receiverState{lastPane: "base", ready: true}
	b.receivers.Store(key, state)

	b.captureAndSend(context.Background())
	time.Sleep(50 * time.Millisecond)

	if len(ms.Texts()) != 0 {
		t.Fatalf("bullet table header should not be sent as text, got %v", ms.Texts())
	}
	if len(ms.Tables()) != 0 {
		t.Fatalf("incomplete table should not be sent yet, got %v", ms.Tables())
	}
	if strings.Join(state.pendingTable, "\n") != "| 维度 | 优点 | 缺点 / 风险 |" {
		t.Fatalf("pendingTable = %v", state.pendingTable)
	}
}

func TestCaptureAndSendMultiReceiver(t *testing.T) {
	tm := &mockTerminal{
		captures: []string{
			"base\nupdate",
		},
	}
	ms := &mockMessenger{}

	b := &Bridge{
		messenger:       ms,
		term:            tm,
		captureInterval: 3 * time.Second,
		sendTimeout:     10 * time.Second,
		historyLines:    2000,
		noisePatterns:   []string{"fluttering", "nesting", "thinking"},
	}

	key1 := receiverKey{id: "user1", kind: "open_id"}
	key2 := receiverKey{id: "user2", kind: "open_id"}
	b.receivers.Store(key1, &receiverState{lastPane: "base", ready: true})
	b.receivers.Store(key2, &receiverState{lastPane: "base", ready: true})

	ctx := context.Background()
	b.captureAndSend(ctx)
	time.Sleep(100 * time.Millisecond)

	if len(ms.Texts()) != 2 {
		t.Errorf("expected 2 messages (1 per receiver), got %d", len(ms.Texts()))
	}
}
