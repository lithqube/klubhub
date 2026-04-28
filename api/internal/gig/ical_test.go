package gig

import (
	"testing"
)

func TestValidateSecret(t *testing.T) {
	tests := []struct {
		name     string
		provided string
		expected string
		want     bool
	}{
		{
			name:     "matching secrets",
			provided: "mysecret",
			expected: "mysecret",
			want:     true,
		},
		{
			name:     "non-matching secrets",
			provided: "mysecret",
			expected: "othersecret",
			want:     false,
		},
		{
			name:     "empty provided",
			provided: "",
			expected: "mysecret",
			want:     false,
		},
		{
			name:     "empty expected",
			provided: "mysecret",
			expected: "",
			want:     false,
		},
		{
			name:     "both empty",
			provided: "",
			expected: "",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateSecret(tt.provided, tt.expected)
			if got != tt.want {
				t.Errorf("ValidateSecret(%q, %q) = %v, want %v", tt.provided, tt.expected, got, tt.want)
			}
		})
	}
}

func TestEscapeICalText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello World"},
		{"Hello;World", "Hello\\;World"},
		{"Hello,World", "Hello\\,World"},
		{"Hello\\World", "Hello\\\\World"},
		{"Hello\nWorld", "Hello\\nWorld"},
		{"", ""},
		{"Multiple;Special,Chars;Here", "Multiple\\;Special\\,Chars\\;Here"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeICalText(tt.input)
			if got != tt.expected {
				t.Errorf("escapeICalText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
