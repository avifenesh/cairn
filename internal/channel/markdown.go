package channel

import (
	"strings"
)

// Normalize converts CommonMark text to platform-specific format.
// Supported targets: "telegram", "discord", "slack", "matrix", "plain".
func Normalize(markdown, target string) string {
	switch target {
	case "telegram":
		return toTelegramV2(markdown)
	case "slack":
		return toSlackMrkdwn(markdown)
	case "matrix":
		return toMatrixHTML(markdown)
	case "discord":
		return markdown // Discord uses standard markdown
	case "plain":
		return stripMarkdown(markdown)
	default:
		return markdown
	}
}

// toTelegramV2 converts CommonMark to Telegram MarkdownV2.
// Telegram V2 uses different bold syntax and requires escaping special chars.
func toTelegramV2(md string) string {
	// Escape special characters that Telegram V2 requires.
	// Must be done BEFORE converting markdown syntax.
	specials := []string{"_", "[", "]", "(", ")", "~", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}

	// First, protect existing markdown syntax by replacing temporarily.
	md = strings.ReplaceAll(md, "**", "\x01BOLD\x01")
	md = strings.ReplaceAll(md, "```", "\x01CODE3\x01")
	md = strings.ReplaceAll(md, "`", "\x01CODE1\x01")

	// Escape specials.
	for _, s := range specials {
		md = strings.ReplaceAll(md, s, "\\"+s)
	}

	// Restore markdown with Telegram V2 syntax.
	md = strings.ReplaceAll(md, "\x01BOLD\x01", "*")    // **bold** → *bold*
	md = strings.ReplaceAll(md, "\x01CODE3\x01", "```") // code blocks stay
	md = strings.ReplaceAll(md, "\x01CODE1\x01", "`")   // inline code stays
	return md
}

// toSlackMrkdwn converts CommonMark to Slack's mrkdwn format.
func toSlackMrkdwn(md string) string {
	// Bold: **text** → *text*
	md = strings.ReplaceAll(md, "**", "*")
	// Links: [text](url) → <url|text>
	md = convertLinks(md, func(text, url string) string {
		return "<" + url + "|" + text + ">"
	})
	return md
}

// toMatrixHTML converts CommonMark to basic HTML for Matrix.
func toMatrixHTML(md string) string {
	// Bold: **text** → <strong>text</strong>
	md = replacePairs(md, "**", "<strong>", "</strong>")
	// Italic: *text* → <em>text</em>  (single asterisk after bold conversion)
	md = replacePairs(md, "*", "<em>", "</em>")
	// Code: `text` → <code>text</code>
	md = replacePairs(md, "`", "<code>", "</code>")
	// Links: [text](url) → <a href="url">text</a>
	md = convertLinks(md, func(text, url string) string {
		return `<a href="` + url + `">` + text + `</a>`
	})
	// Newlines → <br>
	md = strings.ReplaceAll(md, "\n", "<br>\n")
	return md
}

// stripMarkdown removes all markdown formatting for plain text output.
func stripMarkdown(md string) string {
	md = strings.ReplaceAll(md, "**", "")
	md = strings.ReplaceAll(md, "*", "")
	md = strings.ReplaceAll(md, "`", "")
	md = strings.ReplaceAll(md, "```", "")
	md = convertLinks(md, func(text, _ string) string { return text })
	return md
}

// convertLinks finds [text](url) patterns and applies a conversion function.
func convertLinks(md string, convert func(text, url string) string) string {
	var result strings.Builder
	i := 0
	for i < len(md) {
		// Look for [
		if md[i] == '[' {
			// Find matching ]
			closeBracket := strings.Index(md[i:], "](")
			if closeBracket < 0 {
				result.WriteByte(md[i])
				i++
				continue
			}
			closeBracket += i

			// Find closing )
			closeParen := strings.IndexByte(md[closeBracket+2:], ')')
			if closeParen < 0 {
				result.WriteByte(md[i])
				i++
				continue
			}
			closeParen += closeBracket + 2

			text := md[i+1 : closeBracket]
			url := md[closeBracket+2 : closeParen]
			result.WriteString(convert(text, url))
			i = closeParen + 1
		} else {
			result.WriteByte(md[i])
			i++
		}
	}
	return result.String()
}

// replacePairs replaces matched pairs of a delimiter with open/close tags.
func replacePairs(s, delim, open, close string) string {
	count := strings.Count(s, delim)
	if count < 2 {
		return s
	}

	var result strings.Builder
	isOpen := true
	parts := strings.SplitN(s, delim, -1)
	for i, part := range parts {
		result.WriteString(part)
		if i < len(parts)-1 {
			if isOpen {
				result.WriteString(open)
			} else {
				result.WriteString(close)
			}
			isOpen = !isOpen
		}
	}
	return result.String()
}
