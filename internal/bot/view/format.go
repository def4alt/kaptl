package view

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// FormatMoney keeps the ISO currency explicit so values from different
// currencies can never look interchangeable in the UI.
func FormatMoney(value float64, currency string, decimals int) string {
	negative := value < 0
	if negative {
		value = -value
	}

	parts := strings.SplitN(fmt.Sprintf("%.*f", decimals, value), ".", 2)
	digits := parts[0]
	var formatted strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			formatted.WriteByte(',')
		}
		formatted.WriteRune(digit)
	}
	if len(parts) == 2 {
		formatted.WriteByte('.')
		formatted.WriteString(parts[1])
	}

	prefix := strings.ToUpper(currency) + " "
	if negative {
		prefix += "-"
	}
	return prefix + formatted.String()
}

const (
	// mobileWidth is the shared practical width for Telegram mobile messages.
	// It fills the bubble on common phones while leaving room for side padding.
	mobileWidth = 28
	barWidth    = mobileWidth
	cardWidth   = mobileWidth
)

// fitDisplay truncates and right-pads text to an exact terminal display width.
// Unlike len or rune count, runewidth handles wide emoji and CJK characters.
func fitDisplay(text string, width int) string {
	text = sanitizeCodeText(text)
	text = truncateDisplay(text, width)
	return text + strings.Repeat(" ", max(0, width-runewidth.StringWidth(text)))
}

func truncateDisplay(text string, width int) string {
	if runewidth.StringWidth(text) <= width {
		return text
	}
	return runewidth.Truncate(text, width, "…")
}

func sanitizeCodeText(text string) string {
	// A backtick would terminate Telegram's Markdown code block.
	return strings.ReplaceAll(text, "`", "’")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
