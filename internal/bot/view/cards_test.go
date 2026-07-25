package view

import (
	"strings"
	"testing"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/mattn/go-runewidth"
)

func TestFitDisplayUsesVisualWidth(t *testing.T) {
	got := fitDisplay("💳 Monobank", 14)
	if width := runewidth.StringWidth(got); width != 14 {
		t.Fatalf("display width = %d, want 14; %q", width, got)
	}
}

func TestFitDisplayTruncatesLongUnicodeText(t *testing.T) {
	got := fitDisplay("🎮 Entertainment & Hobbies", 14)
	if width := runewidth.StringWidth(got); width != 14 {
		t.Fatalf("display width = %d, want 14; %q", width, got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("expected ellipsis in %q", got)
	}
}

func TestRenderCardsUsesSingleCodeBlock(t *testing.T) {
	got := RenderCards([]Card{
		NewCard("💳 Monobank", "€20,000"),
		NewCard("💵 Cash", "€0"),
	})

	if strings.Count(got, "```") != 2 {
		t.Fatalf("expected one Markdown code block, got %q", got)
	}

	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "│ ") {
			if width := runewidth.StringWidth(line); width != cardWidth+4 {
				t.Fatalf("card line width = %d, want %d; %q", width, cardWidth+4, line)
			}
		}
	}
}

func TestAccountsUsesCardRendererAndBottomTotal(t *testing.T) {
	got := Accounts([]models.Account{
		{Name: "Monobank", Emoji: "💳", Balance: 20000},
		{Name: "Cash", Emoji: "💵", Balance: 0},
	})

	checks := []string{
		"💰 *Accounts*",
		"```\n┌────────────────┐",
		"│ 💳 Monobank    │",
		"│ €20,000        │",
		"*Total: €20,000*",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("Accounts output missing %q:\n%s", want, got)
		}
	}

	if !strings.HasSuffix(got, "*Total: €20,000*") {
		t.Fatalf("total must stay at the bottom for mobile-first reading:\n%s", got)
	}
}

func TestFormatAmount(t *testing.T) {
	cases := map[float64]string{
		0:      "€0",
		20000:  "€20,000",
		-4950:  "-€4,950",
		2830.4: "€2,830",
	}
	for value, want := range cases {
		if got := formatAmount(value); got != want {
			t.Errorf("formatAmount(%v) = %q, want %q", value, got, want)
		}
	}
}
