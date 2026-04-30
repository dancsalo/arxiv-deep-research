package memoryclient

import (
	"database/sql"
	"strings"
	"testing"
)

func TestValidateTitle(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		wantErr string
	}{
		{"empty", "", "title too short (0 words"},
		{"two words", "two words", "title too short (2 words"},
		{"three words (minimum)", "three word title", ""},
		{"normal title", "Hook timeout breaks npm install", ""},
		{"fifteen words (maximum)", "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen", ""},
		{"sixteen words", "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen", "title too long (16 words"},
		{"extra whitespace", "  spaced   out   title  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTitle(tt.title)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestEstimateTokensFast(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"24 chars", "hello world test string!", 6},
		{"400 chars", strings.Repeat("a", 400), 100},
		{"short", "hi", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokensFast(tt.text)
			if got != tt.want {
				t.Errorf("estimateTokensFast(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestMemoryTypeToEmoji(t *testing.T) {
	mappings := map[string]string{
		"session-goal": "🎯",
		"gotcha":       "🔴",
		"problem-fix":  "🟡",
		"how-it-works": "🔵",
		"what-changed": "🟢",
		"discovery":    "🟣",
		"why-it-exists": "🟠",
		"decision":     "🟤",
		"trade-off":    "⚖️",
	}
	for key, want := range mappings {
		t.Run(key, func(t *testing.T) {
			got := memoryTypeToEmoji(key)
			if got != want {
				t.Errorf("memoryTypeToEmoji(%q) = %q, want %q", key, got, want)
			}
		})
	}

	t.Run("unknown returns key", func(t *testing.T) {
		got := memoryTypeToEmoji("unknown-type")
		if got != "unknown-type" {
			t.Errorf("expected key back for unknown type, got %q", got)
		}
	})
}

func TestValidMemoryTypes(t *testing.T) {
	for _, mt := range []string{"session-goal", "gotcha", "problem-fix", "how-it-works", "what-changed", "discovery", "why-it-exists", "decision", "trade-off"} {
		if !validMemoryTypes[mt] {
			t.Errorf("expected %q to be a valid memory type", mt)
		}
	}
	if validMemoryTypes["random-garbage"] {
		t.Error("expected 'random-garbage' to be invalid")
	}
}

func TestPgVectorLiteral(t *testing.T) {
	vec := []float32{0.1, 0.2, 0.3}
	got := pgVectorLiteral(vec)
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("expected brackets, got %q", got)
	}
	if !strings.Contains(got, "0.1") {
		t.Errorf("expected 0.1 in output, got %q", got)
	}
}

func TestNullString(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ns := nullString("")
		if ns.Valid {
			t.Error("expected invalid for empty string")
		}
	})
	t.Run("non-empty", func(t *testing.T) {
		ns := nullString("hello")
		if !ns.Valid || ns.String != "hello" {
			t.Errorf("expected valid 'hello', got %v", ns)
		}
	})
	t.Run("implements NullString", func(t *testing.T) {
		var _ sql.NullString = nullString("test")
	})
}
