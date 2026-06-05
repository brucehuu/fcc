package bot

import (
	"fmt"
	"strings"
	"testing"
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
