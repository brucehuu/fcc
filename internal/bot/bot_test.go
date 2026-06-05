package bot

import (
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
			name:     "empty header",
			input:    "| | |\n| --- | --- |",
			wantErr:  false,
			wantRows: 0,
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
			elements, ok := card["elements"].([]map[string]interface{})
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
			columns, ok := table["columns"].([]map[string]interface{})
			if !ok || len(columns) != len(parseMarkdownTableCells(strings.Split(tt.input, "\n")[0])) {
				t.Fatalf("columns = %v", table["columns"])
			}
			rows, ok := table["rows"].([]map[string]interface{})
			if !ok || len(rows) != tt.wantRows {
				t.Errorf("rows = %v, want %d rows", table["rows"], tt.wantRows)
			}
		})
	}
}
