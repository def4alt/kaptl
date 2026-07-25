package view

import (
	"fmt"
	"strings"
)

// Card is presentation data only. RenderCards owns all borders, sizing,
// truncation, padding, and Markdown code-block wrapping.
type Card struct {
	Lines []string
}

func NewCard(lines ...string) Card {
	return Card{Lines: lines}
}

// RenderCards renders one monospace block. Telegram's proportional font cannot
// align box-drawing characters, so every card stack must stay inside this block.
func RenderCards(cards []Card) string {
	if len(cards) == 0 {
		return ""
	}

	border := strings.Repeat("─", cardWidth+2)
	var b strings.Builder
	b.WriteString("```\n")

	for i, card := range cards {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("┌" + border + "┐\n")
		for _, line := range card.Lines {
			b.WriteString(fmt.Sprintf("│ %s │\n", fitDisplay(line, cardWidth)))
		}
		b.WriteString("└" + border + "┘\n")
	}

	b.WriteString("```")
	return b.String()
}
