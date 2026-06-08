package bot

import (
	"testing"
)

func TestTableColumnWidth(t *testing.T) {
	tests := []struct {
		name     string
		index    int
		colCount int
		want     string
	}{
		{"single column", 0, 1, "100%"},
		{"two columns first", 0, 2, "50%"},
		{"two columns second", 1, 2, "50%"},
		{"three columns first", 0, 3, "34%"},
		{"three columns second", 1, 3, "33%"},
		{"three columns third", 2, 3, "33%"},
		{"four columns", 0, 4, "25%"},
		{"five columns first", 0, 5, "20%"},
		{"five columns last", 4, 5, "20%"},
		{"seven columns remainder", 0, 7, "15%"},
		{"seven columns no remainder", 2, 7, "14%"},
		{"zero columns fallback", 0, 0, "100%"},
		{"negative columns fallback", 0, -1, "100%"},
		{"index beyond remainder", 5, 7, "14%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tableColumnWidth(tt.index, tt.colCount)
			if got != tt.want {
				t.Errorf("tableColumnWidth(%d, %d) = %q, want %q", tt.index, tt.colCount, got, tt.want)
			}
		})
	}
}

func TestBuildTableCardEmptyHeader(t *testing.T) {
	// Header parses to empty cells after trimming pipes and spaces.
	_, err := buildTableCard("|   |   |\n| --- | --- |")
	if err == nil {
		t.Fatal("buildTableCard() expected error for empty header, got nil")
	}
}

func TestBuildTableCardOnlySeparatorAndBlank(t *testing.T) {
	// Table with header, separator, and a line that parses to zero cells.
	// "|" splits to ["", ""]; after trimming, both ends are empty and skipped,
	// resulting in zero cells, so the line is ignored and no data rows remain.
	_, err := buildTableCard("| A | B |\n| --- | --- |\n|")
	if err == nil {
		t.Fatal("buildTableCard() expected error when all data rows are empty, got nil")
	}
}

func TestBuildTableCardIgnoresEmptyLines(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n\n| 1 | 2 |\n"
	card, err := buildTableCard(input)
	if err != nil {
		t.Fatalf("buildTableCard() error = %v", err)
	}
	body := card["body"].(map[string]interface{})
	elements := body["elements"].([]map[string]interface{})
	table := elements[0]
	rows := table["rows"].([]map[string]interface{})
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
}
