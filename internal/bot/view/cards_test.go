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
		{Name: "Monobank", Emoji: "💳", Balance: 20000, Currency: "EUR"},
		{Name: "Privat", Emoji: "💵", Balance: 41000, Currency: "UAH"},
	})

	checks := []string{
		"💰 *Accounts*",
		"```\n┌" + strings.Repeat("─", cardWidth+2) + "┐",
		"│ 💳 Monobank",
		"│ EUR 20,000",
		"│ UAH 41,000",
		"*EUR total: EUR 20,000*",
		"*UAH total: UAH 41,000*",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("Accounts output missing %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "Total: EUR 61,000") || strings.Contains(got, "Total: UAH 61,000") {
		t.Fatalf("account totals must not add different currencies:\n%s", got)
	}
}

func TestSummaryUsesFullWidthBar(t *testing.T) {
	got := Summary([]models.BudgetRow{{
		Name:      "Groceries",
		Emoji:     "🛒",
		Currency:  "UAH",
		Spent:     50,
		Available: 100,
		Remaining: 50,
	}}, []models.CurrencyAmount{{Currency: "UAH", Amount: 200}})

	bar := strings.Repeat("█", barWidth/2) + strings.Repeat("░", barWidth/2)
	if !strings.Contains(got, "🛒 Groceries · 50%") {
		t.Fatalf("summary missing percentage on category line:\n%s", got)
	}
	if !strings.Contains(got, "`"+bar+"`") {
		t.Fatalf("summary missing %d-column bar:\n%s", barWidth, got)
	}
}

func TestSummaryKeepsCurrencyTotalsSeparate(t *testing.T) {
	got := Summary([]models.BudgetRow{
		{Name: "Food", Emoji: "🍞", Currency: "EUR", Spent: 19, Available: 100, Remaining: 81},
		{Name: "Food", Emoji: "🍞", Currency: "UAH", Spent: 2409, Available: 0, Remaining: -2409},
	}, []models.CurrencyAmount{
		{Currency: "EUR", Amount: 4},
		{Currency: "UAH", Amount: 14},
	})

	for _, want := range []string{"EUR 19 / EUR 100", "UAH 2,409 / UAH 0", "EUR total: EUR 19 / EUR 100", "UAH total: UAH 2,409 / UAH 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "2,428") || strings.Contains(got, "2,509") {
		t.Fatalf("summary mixed EUR and UAH values:\n%s", got)
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[float64]string{
		0:      "EUR 0",
		20000:  "EUR 20,000",
		-4950:  "EUR -4,950",
		2830.4: "EUR 2,830",
	}
	for value, want := range cases {
		if got := FormatMoney(value, "eur", 0); got != want {
			t.Errorf("FormatMoney(%v) = %q, want %q", value, got, want)
		}
	}
}

func TestRecentUsesTransactionCurrency(t *testing.T) {
	got := Recent([]models.Transaction{{
		Type: "expense", Amount: 2409, Currency: "UAH", AccountName: "Privat",
	}})
	if !strings.Contains(got, "UAH 2,409") {
		t.Fatalf("recent transaction missing UAH amount:\n%s", got)
	}
	if strings.Contains(got, "€2,409") {
		t.Fatalf("recent transaction rendered as EUR:\n%s", got)
	}
}

func TestBudgetsRenderEachCurrency(t *testing.T) {
	got := Budgets(
		[]models.Category{{ID: 1, Name: "Food", Emoji: "🍞"}},
		[]models.Budget{
			{CategoryID: 1, Currency: "EUR", Amount: 100, IntervalMonths: 1},
			{CategoryID: 1, Currency: "UAH", Amount: 5000, IntervalMonths: 1},
		},
	)
	for _, want := range []string{"EUR 100", "UAH 5,000"} {
		if !strings.Contains(got, want) {
			t.Errorf("budgets missing %q:\n%s", want, got)
		}
	}
}

func TestEntityResponsesHaveNoCompatibilityArtifacts(t *testing.T) {
	if got := Created("🛒", "Groceries", ""); got != "✅ Created category: 🛒 *Groceries*" {
		t.Fatalf("Created() = %q", got)
	}
	if got := Deleted("", "Needs", "group"); got != "✅ Deleted group: Needs" {
		t.Fatalf("Deleted() = %q", got)
	}
}
