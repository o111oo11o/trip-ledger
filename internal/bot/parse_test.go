package bot

import (
	"testing"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"45.50", 4550, false},
		{"100", 10000, false},
		{"0.01", 1, false},
		{"0.1", 10, false},
		{"1000.99", 100099, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-5", 0, true},
	}
	for _, tt := range tests {
		got, err := parseAmount(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseAmount(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAmount(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("parseAmount(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		day     int
		month   int
	}{
		{"15-03-24", false, 15, 3},
		{"01-12", false, 1, 12},
		{"invalid", true, 0, 0},
		{"01-02-03-04", true, 0, 0},
	}
	for _, tt := range tests {
		d, err := parseDate(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseDate(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDate(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if d.Day() != tt.day || int(d.Month()) != tt.month {
			t.Errorf("parseDate(%q) = %v, want day=%d month=%d", tt.input, d, tt.day, tt.month)
		}
	}
}

func TestIsUsername(t *testing.T) {
	if !isUsername("@alice") {
		t.Error("expected @alice to be username")
	}
	if isUsername("alice") {
		t.Error("expected alice not to be username")
	}
	if isUsername("@") {
		t.Error("expected @ alone not to be username")
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("/split @bob 100 EUR dinner out")
	expected := []string{"@bob", "100", "EUR", "dinner", "out"}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
	for i, tok := range tokens {
		if tok != expected[i] {
			t.Errorf("token[%d] = %q, want %q", i, tok, expected[i])
		}
	}
}

func TestIsCurrency(t *testing.T) {
	if !isCurrency("USD") {
		t.Error("USD should be valid")
	}
	if !isCurrency("eur") {
		t.Error("eur should be valid (case insensitive)")
	}
	if isCurrency("US") {
		t.Error("US should be invalid (too short)")
	}
	if isCurrency("1234") {
		t.Error("1234 should be invalid")
	}
}
