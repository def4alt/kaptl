package view

import (
	"strings"
	"testing"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/mattn/go-runewidth"
)

func TestFitDisplayUsesVisualWidth(t *testing.T) {
	got := fitDisplay("💳 Monobank", cardWidth)
	if width := runewidth.StringWidth(got); width != cardWidth {
		t.Fatalf("display width = %d, want %d; %q", width, cardWidth, got)
	}
}

func TestFitDisplayTruncatesLongUnicodeText(t *testing.T) {
	got := fitDisplay("🎮 Entertainment, Hobbies & Subscriptions", cardWidth)
	if width := runewidth.StringWidth(got); width != cardWidth {
		t.Fatalf("display width = %d, want %d; %q", width, cardWidth, got)
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
		"```\n┌" + strings.Repeat("─", cardWidth+2) + "┐",
		"│ 💳 Monobank",
		"│ €20,000",
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

func TestSummaryUsesFullWidthBar(t *testing.T) {
	got := Summary([]models.BudgetRow{{
		Name:      "Groceries",
		Emoji:     "🛒",
		Spent:     50,
		Available: 100,
		Remaining: 50,
	}}, 200)

	bar := strings.Repeat("█", barWidth/2) + strings.Repeat("░", barWidth/2)
	if !strings.Contains(got, "🛒 Groceries · 50%") {
		t.Fatalf("summary missing percentage on category line:\n%s", got)
	}
	if !strings.Contains(got, "`"+bar+"`") {
		t.Fatalf("summary missing %d-column bar:\n%s", barWidth, got)
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
