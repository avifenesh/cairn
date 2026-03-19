package voice

import "testing"

func TestSanitizeForSpeech(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips URLs",
			input:    "Check out https://github.com/avifenesh/cairn for more info.",
			expected: "Check out for more info.",
		},
		{
			name:     "converts markdown links to text",
			input:    "See the [documentation](https://docs.example.com) for details.",
			expected: "See the documentation for details.",
		},
		{
			name:     "removes code blocks",
			input:    "Here's how:\n```go\nfmt.Println(\"hello\")\n```\nThat's it.",
			expected: "Here's how: That's it.",
		},
		{
			name:     "strips inline code backticks",
			input:    "Run the `go build` command to compile.",
			expected: "Run the go build command to compile.",
		},
		{
			name:     "removes bold/italic markdown",
			input:    "This is **important** and *emphasized* text.",
			expected: "This is important and emphasized text.",
		},
		{
			name:     "removes heading markers",
			input:    "## Section Title\nSome content here.",
			expected: "Section Title\nSome content here.",
		},
		{
			name:     "cleans list markers",
			input:    "- Item one\n- Item two\n* Item three",
			expected: "Item one\nItem two\nItem three",
		},
		{
			name:     "removes images, keeps alt text",
			input:    "Look at this ![screenshot](https://img.example.com/pic.png) image.",
			expected: "Look at this screenshot image.",
		},
		{
			name:     "handles mixed content",
			input:    "## Update\n**Bold**: Check [link](https://x.com). Run `cmd`.\n```\ncode\n```\nDone!",
			expected: "Update\nBold: Check link. Run cmd. Done!",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text unchanged",
			input:    "Hello, this is a normal sentence.",
			expected: "Hello, this is a normal sentence.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForSpeech(tt.input)
			if got != tt.expected {
				t.Errorf("\ninput:    %q\nexpected: %q\ngot:      %q", tt.input, tt.expected, got)
			}
		})
	}
}
