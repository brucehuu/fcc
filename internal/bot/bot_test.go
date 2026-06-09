package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMarkdownTableCells(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"basic two columns", "| A | B |", []string{"A", "B"}},
		{"three columns", "| 1 | 2 | 3 |", []string{"1", "2", "3"}},
		{"empty cell preserved", "| A |  | B |", []string{"A", "", "B"}},
		{"all empty", "| | |", []string{"", ""}},
		{"no pipes", "hello", []string{"hello"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMarkdownTableCells(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseMarkdownTableCells(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseMarkdownTableCells(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsSeparatorLine(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"| --- | --- |", true},
		{"| :--- | ---: |", true},
		{"| :---: |", true},
		{"| -- |", false},  // too short
		{"| -+- |", false}, // invalid chars
		{"hello", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isSeparatorLine(tt.input)
		if got != tt.want {
			t.Errorf("isSeparatorLine(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBuildTableCard(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		wantRows int
	}{
		{
			name:     "normal table",
			input:    "| A | B |\n| --- | --- |\n| 1 | 2 |",
			wantErr:  false,
			wantRows: 1,
		},
		{
			name:    "too short",
			input:   "| A | B |",
			wantErr: true,
		},
		{
			name:    "header only",
			input:   "| A | B |\n| --- | --- |",
			wantErr: true,
		},
		{
			name:     "column padding",
			input:    "| A | B |\n| --- | --- |\n| 1 |",
			wantErr:  false,
			wantRows: 1,
		},
		{
			name:     "column truncation",
			input:    "| A | B |\n| --- | --- |\n| 1 | 2 | 3 |",
			wantErr:  false,
			wantRows: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card, err := buildTableCard(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildTableCard() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if card["schema"] != "2.0" {
				t.Fatalf("schema = %v, want 2.0", card["schema"])
			}
			body, ok := card["body"].(map[string]interface{})
			if !ok {
				t.Fatalf("body not found or wrong type: %v", card["body"])
			}
			elements, ok := body["elements"].([]map[string]interface{})
			if !ok {
				t.Fatal("elements not found or wrong type")
			}
			if len(elements) != 1 {
				t.Fatalf("len(elements) = %d, want 1 table element", len(elements))
			}
			table := elements[0]
			if table["tag"] != "table" {
				t.Fatalf("table tag = %v, want table", table["tag"])
			}
			if table["row_height"] != "auto" {
				t.Fatalf("row_height = %v, want auto", table["row_height"])
			}
			if table["row_max_height"] != "360px" {
				t.Fatalf("row_max_height = %v, want 360px", table["row_max_height"])
			}
			headerStyle, ok := table["header_style"].(map[string]interface{})
			if !ok {
				t.Fatalf("header_style = %v", table["header_style"])
			}
			if headerStyle["lines"] != 2 {
				t.Fatalf("header lines = %v, want 2", headerStyle["lines"])
			}
			columns, ok := table["columns"].([]map[string]interface{})
			if !ok || len(columns) != len(parseMarkdownTableCells(strings.Split(tt.input, "\n")[0])) {
				t.Fatalf("columns = %v", table["columns"])
			}
			for _, column := range columns {
				if column["width"] == "auto" {
					t.Fatalf("column width should use percentage, got auto: %v", column)
				}
			}
			rows, ok := table["rows"].([]map[string]interface{})
			if !ok || len(rows) != tt.wantRows {
				t.Errorf("rows = %v, want %d rows", table["rows"], tt.wantRows)
			}
		})
	}
}

func TestBuildTableCardUsesPercentageColumnWidths(t *testing.T) {
	card, err := buildTableCard("| A | B | C | D |\n| --- | --- | --- | --- |\n| 1 | 2 | 3 | 4 |")
	if err != nil {
		t.Fatalf("buildTableCard() error = %v", err)
	}
	body := card["body"].(map[string]interface{})
	elements := body["elements"].([]map[string]interface{})
	table := elements[0]
	columns := table["columns"].([]map[string]interface{})
	want := []string{"25%", "25%", "25%", "25%"}
	for i, column := range columns {
		if column["width"] != want[i] {
			t.Fatalf("column %d width = %v, want %s", i, column["width"], want[i])
		}
	}
}

func TestBuildTableCardShowsAllRowsOnOnePage(t *testing.T) {
	lines := []string{
		"| A | B |",
		"| --- | --- |",
	}
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf("| row-%02d | value-%02d |", i, i))
	}
	card, err := buildTableCard(strings.Join(lines, "\n"))
	if err != nil {
		t.Fatalf("buildTableCard() error = %v", err)
	}
	body := card["body"].(map[string]interface{})
	elements := body["elements"].([]map[string]interface{})
	table := elements[0]
	if table["page_size"] != 12 {
		t.Fatalf("page_size = %v, want 12", table["page_size"])
	}
	rows := table["rows"].([]map[string]interface{})
	if len(rows) != 12 {
		t.Fatalf("rows = %d, want 12", len(rows))
	}
}

func TestPtrStr(t *testing.T) {
	s := "hello"
	if got := ptrStr(&s); got != "hello" {
		t.Errorf("ptrStr(&s) = %q, want hello", got)
	}
	if got := ptrStr(nil); got != "" {
		t.Errorf("ptrStr(nil) = %q, want empty", got)
	}
}

func TestParseMessageText(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)

	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{"simple text", `{"text":"hello world"}`, "hello world", false},
		{"text with spaces", `{"text":"  trim me  "}`, "trim me", false},
		{"empty text", `{"text":""}`, "", false},
		{"invalid json", `not json`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := b.parseMessage(nil, "text", tt.content, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMessageImageNoKey(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	got, err := b.parseMessage(nil, "image", `{"image_key":""}`, nil)
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	if got != "" {
		t.Errorf("parseMessage() = %q, want empty", got)
	}
}

func TestCleanupOldImages(t *testing.T) {
	origDir := ".fcc/images"

	if err := os.MkdirAll(origDir, 0755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	defer os.RemoveAll(".fcc")

	// Create an old file.
	oldFile := filepath.Join(origDir, "old.png")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	// Set mod time to 2 days ago.
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes error = %v", err)
	}

	// Create a new file.
	newFile := filepath.Join(origDir, "new.png")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	b := New("test-id", "test-secret", nil, 3, 0, 0)
	if err := b.CleanupOldImages(24 * time.Hour); err != nil {
		t.Fatalf("CleanupOldImages() error = %v", err)
	}

	// Old file should be removed.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should be removed")
	}
	// New file should remain.
	if _, err := os.Stat(newFile); err != nil {
		t.Error("new file should remain")
	}
}

func TestCleanupOldImagesNoDir(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	// Clean up any existing .fcc/images first.
	os.RemoveAll(".fcc")
	if err := b.CleanupOldImages(24 * time.Hour); err != nil {
		t.Fatalf("CleanupOldImages() error = %v", err)
	}
}

func TestDownloadImageEmptyParams(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	_, err := b.downloadImage(nil, "", "key")
	if err == nil {
		t.Error("downloadImage() expected error for empty messageID")
	}
	_, err = b.downloadImage(nil, "msg", "")
	if err == nil {
		t.Error("downloadImage() expected error for empty imageKey")
	}
}

func TestParseMessageImage(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	ctx := context.Background()

	// Empty image key should return empty string.
	got, err := b.parseMessage(ctx, "image", `{"image_key":""}`, nil)
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	if got != "" {
		t.Errorf("parseMessage() = %q, want empty", got)
	}

	// Invalid JSON should return error.
	_, err = b.parseMessage(ctx, "image", `not json`, nil)
	if err == nil {
		t.Error("parseMessage() expected error for invalid JSON")
	}

	// Non-image type should return empty.
	got, err = b.parseMessage(ctx, "file", `{"text":"hello"}`, nil)
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	if got != "" {
		t.Errorf("parseMessage() = %q, want empty", got)
	}
}

func TestParseMessageImageDownload(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	ctx := context.Background()
	msgID := "msg-123"

	// Non-empty image key will attempt download and fail, returning fallback text.
	got, err := b.parseMessage(ctx, "image", `{"image_key":"img-key"}`, &msgID)
	if err != nil {
		t.Fatalf("parseMessage() error = %v", err)
	}
	if got == "" {
		t.Error("parseMessage() should return fallback message for failed download")
	}
}

func TestSendInteractiveTableInvalid(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	ctx := context.Background()

	// Invalid markdown table (no separator).
	err := b.SendInteractiveTable(ctx, "open_id", "test", "| A | B |")
	if err == nil {
		t.Error("SendInteractiveTable() expected error for invalid table")
	}
}

func TestSendInteractiveTableEmpty(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	ctx := context.Background()

	// Empty markdown.
	err := b.SendInteractiveTable(ctx, "open_id", "test", "")
	if err == nil {
		t.Error("SendInteractiveTable() expected error for empty input")
	}
}

func TestSendInteractiveTableValid(t *testing.T) {
	b := New("test-id", "test-secret", nil, 1, 10*time.Millisecond, 100*time.Millisecond)
	ctx := context.Background()

	// Valid markdown table — will fail on HTTP but covers the sendWithRetry path.
	err := b.SendInteractiveTable(ctx, "open_id", "test", "| A | B |\n| --- | --- |\n| 1 | 2 |")
	if err == nil {
		t.Error("SendInteractiveTable() expected error for unauthenticated bot")
	}
}

func TestBotClose(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	// Should not panic.
	b.Close()
	// Double close should be safe.
	b.Close()
}

func TestSendWelcomeNilBot(t *testing.T) {
	// Bot without onMessage callback.
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	ctx := context.Background()

	// This will fail on HTTP call but covers the function entry.
	// We just verify it doesn't panic with nil callback.
	_ = b.SendWelcome(ctx, "open_id", "test")
}

func TestSendWelcomeEmptyTarget(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 0, 0)
	ctx := context.Background()

	err := b.SendWelcome(ctx, "", "hello")
	if err == nil {
		t.Fatal("SendWelcome('') expected error")
	}
	err = b.SendWelcome(ctx, "   ", "hello")
	if err == nil {
		t.Fatal("SendWelcome('   ') expected error")
	}
}

func TestSendTextTimeout(t *testing.T) {
	b := New("test-id", "test-secret", nil, 1, 10*time.Millisecond, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	err := b.SendText(ctx, "open_id", "test", "hello")
	if err == nil {
		t.Fatal("SendText expected error with short timeout")
	}
}

func TestSendMarkdownTimeout(t *testing.T) {
	b := New("test-id", "test-secret", nil, 1, 10*time.Millisecond, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := b.SendMarkdown(ctx, "open_id", "test", "hello")
	if err == nil {
		t.Fatal("SendMarkdown expected error with short timeout")
	}
}

func TestUpdateMessageTimeout(t *testing.T) {
	b := New("test-id", "test-secret", nil, 1, 10*time.Millisecond, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	err := b.UpdateMessage(ctx, "msg-id", "hello")
	if err == nil {
		t.Fatal("UpdateMessage expected error with short timeout")
	}
}

func TestSendWithRetrySuccess(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 10*time.Millisecond, 100*time.Millisecond)
	ctx := context.Background()

	callCount := 0
	err := b.sendWithRetry(ctx, "test", func(ctx context.Context) error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("sendWithRetry() error = %v", err)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestSendWithRetryEventualSuccess(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, 10*time.Millisecond, 100*time.Millisecond)
	ctx := context.Background()

	callCount := 0
	err := b.sendWithRetry(ctx, "test", func(ctx context.Context) error {
		callCount++
		if callCount < 2 {
			return fmt.Errorf("transient error")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sendWithRetry() error = %v", err)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestSendWithRetryExhausted(t *testing.T) {
	b := New("test-id", "test-secret", nil, 2, 10*time.Millisecond, 100*time.Millisecond)
	ctx := context.Background()

	callCount := 0
	err := b.sendWithRetry(ctx, "test", func(ctx context.Context) error {
		callCount++
		return fmt.Errorf("persistent error")
	})
	if err == nil {
		t.Fatal("sendWithRetry() expected error")
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestSendWithRetryContextCancel(t *testing.T) {
	b := New("test-id", "test-secret", nil, 3, time.Hour, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := b.sendWithRetry(ctx, "test", func(ctx context.Context) error {
		callCount++
		return fmt.Errorf("error")
	})
	if err == nil {
		t.Fatal("sendWithRetry() expected error")
	}
	if callCount < 1 {
		t.Error("expected at least one attempt")
	}
}

func TestBuildInteractiveCardUsesMarkdownComponent(t *testing.T) {
	card := buildInteractiveCard("```json\n{}\n```")
	elements, ok := card["elements"].([]map[string]interface{})
	if !ok || len(elements) != 1 {
		t.Fatalf("elements = %v", card["elements"])
	}
	if elements[0]["tag"] != "markdown" {
		t.Fatalf("tag = %v, want markdown", elements[0]["tag"])
	}
	if elements[0]["content"] != "```json\n{}\n```" {
		t.Fatalf("content = %v", elements[0]["content"])
	}
}
