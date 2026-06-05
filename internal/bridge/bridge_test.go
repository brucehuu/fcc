package bridge

import (
	"context"
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
		{"-claude", "claude", false}, // 选项被跳过
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
	input := "| A | B |\n| 1 | 2 |\nplain text\nanother line"
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

func TestSplitDiffIntoBlocksEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		isTable0 bool
	}{
		{"empty", "", 0, false},
		{"all text", "hello\nworld", 1, false},
		{"all table", "| A | B |\n| 1 | 2 |", 1, true},
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
func (m *mockMessenger) CleanupOldImages(maxAge time.Duration) error { return nil }
func (m *mockMessenger) Close() {}

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
	captures []string
	index    int
	mu       sync.Mutex
}

func (t *mockTerminal) Start(command, workDir string) error { return nil }
func (t *mockTerminal) SendKeys(text string) error       { return nil }
func (t *mockTerminal) SendSpecialKey(key string) error  { return nil }
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
func (t *mockTerminal) WaitReady() error    { return nil }
func (t *mockTerminal) Kill() error         { return nil }
func (t *mockTerminal) HasSession() bool    { return true }
func (t *mockTerminal) IsAvailable() bool   { return true }

func TestCaptureAndSend(t *testing.T) {
	tm := &mockTerminal{
		captures: []string{
			"hello\nworld",          // tick 1: baseline
			"hello\nworld\nfoo",     // tick 2: diff = foo
			"hello\nworld\nfoo",     // tick 3: no diff
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
			"header\n│ A │ B │\n│ 1 │ 2 │",
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

	// 表格现在通过 SendMarkdown 发送，不再走 SendInteractiveTable
	if len(ms.Tables()) != 0 {
		t.Errorf("expected 0 table messages (now sent as markdown), got %d", len(ms.Tables()))
	}
	texts := ms.Texts()
	if len(texts) != 1 {
		t.Errorf("expected 1 markdown message, got %d", len(texts))
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
