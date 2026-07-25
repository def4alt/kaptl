package view

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

const (
	// mobileWidth is the shared practical width for Telegram mobile messages.
	// It fills the bubble on common phones while leaving room for side padding.
	mobileWidth = 28
	barWidth    = mobileWidth
	cardWidth   = mobileWidth
)

// formatAmount formats whole-euro values consistently across every view.
func formatAmount(value float64) string {
	negative := value < 0
	if negative {
		value = -value
	}

	digits := fmt.Sprintf("%.0f", value)
	var formatted strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			formatted.WriteByte(',')
		}
		formatted.WriteRune(digit)
	}

	if negative {
		return "-€" + formatted.String()
	}
	return "€" + formatted.String()
}

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
