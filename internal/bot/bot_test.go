package bot

import (
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
		{"| -- |", false},    // too short
		{"| -+- |", false},   // invalid chars
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

func TestBuildTableRow(t *testing.T) {
	cells := []string{"A", "B", "C"}
	row := buildTableRow(cells, "grey")

	if row["tag"] != "column_set" {
		t.Errorf("tag = %v, want column_set", row["tag"])
	}
	if row["background_style"] != "grey" {
		t.Errorf("background_style = %v, want grey", row["background_style"])
	}

	cols, ok := row["columns"].([]map[string]interface{})
	if !ok || len(cols) != 3 {
		t.Fatalf("columns = %v, want 3 columns", row["columns"])
	}
	for i, c := range cols {
		if c["tag"] != "column" {
			t.Errorf("column[%d].tag = %v, want column", i, c["tag"])
		}
		if c["weight"] != 1 {
			t.Errorf("column[%d].weight = %v, want 1", i, c["weight"])
		}
	}
}

func TestBuildTableCard(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantLen int // expected elements length
	}{
		{
			name:    "normal table",
			input:   "| A | B |\n| --- | --- |\n| 1 | 2 |",
			wantErr: false,
			wantLen: 2,
		},
		{
			name:    "too short",
			input:   "| A | B |",
			wantErr: true,
		},
		{
			name:    "empty header",
			input:   "| | |\n| --- | --- |",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "column padding",
			input:   "| A | B |\n| --- | --- |\n| 1 |",
			wantErr: false,
			wantLen: 2,
		},
		{
			name:    "column truncation",
			input:   "| A | B |\n| --- | --- |\n| 1 | 2 | 3 |",
			wantErr: false,
			wantLen: 2,
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
			if len(elements) != tt.wantLen {
				t.Errorf("len(elements) = %d, want %d", len(elements), tt.wantLen)
			}
		})
	}
}
