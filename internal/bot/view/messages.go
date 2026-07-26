package view

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/shopspring/decimal"
)

// ─── Main messages ─────────────────────────────────────────

func Welcome() string {
	return "💰 *Kaptl* — expense tracker\n\nTrack your spending like a pro."
}

func Help() string {
	return `*Commands:*
/start – Main menu
/cat add 🍞 Name – Create category
/cat rm Name – Delete category
/acc add 💳 Name [currency] – Create account
/budget set Name amount [interval] – Set EUR reporting budget
/group add 📁 Name – Create category group
/group rm Name – Delete group
/move amount from Account to Account – Transfer

*Quick expense:*
Tap "➖ Expense" → pick category → type amount → pick account → done!

*Currency defaults to EUR.*`
}

// ─── Summary ──────────────────────────────────────────────

func Summary(rows []models.BudgetRow, rta []models.CurrencyAmount) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📊 *%s*\n", time.Now().Format("January 2006")))

	sort.Slice(rta, func(i, j int) bool { return rta[i].Currency < rta[j].Currency })
	for _, amount := range rta {
		color := "🟢"
		if amount.Amount.IsNegative() {
			color = "🔴"
		}
		b.WriteString(fmt.Sprintf("💵 Ready to Assign: %s %s\n", color, FormatMoney(amount.Amount, amount.Currency, 0)))
	}

	totalSpent := make(map[string]decimal.Decimal)
	totalBudget := make(map[string]decimal.Decimal)
	lastGroup := ""
	first := true

	for _, r := range rows {
		if r.GroupName != "" && r.GroupName != lastGroup {
			lastGroup = r.GroupName
			if !first {
				b.WriteString("\n")
			}
			first = false
			b.WriteString(fmt.Sprintf("\n🏠 *%s*\n", lastGroup))
		}

		ratio := decimal.Zero
		if r.Available.IsPositive() {
			ratio = r.Spent.Div(r.Available)
			if ratio.GreaterThan(decimal.NewFromInt(1)) {
				ratio = decimal.NewFromInt(1)
			}
			if ratio.IsNegative() {
				ratio = decimal.Zero
			}
		}
		filled := int(ratio.Mul(decimal.NewFromInt(barWidth)).IntPart())
		percent := ratio.Mul(decimal.NewFromInt(100)).IntPart()
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

		left := ""
		if r.Remaining.IsPositive() {
			left = fmt.Sprintf("+%s left", FormatMoney(r.Remaining, r.Currency, 0))
		} else if r.Remaining.IsNegative() {
			left = fmt.Sprintf("-%s over", FormatMoney(r.Remaining.Abs(), r.Currency, 0))
		} else {
			left = FormatMoney(decimal.Zero, r.Currency, 0) + " left"
		}

		name := truncateDisplay(fmt.Sprintf("%s %s", r.Emoji, r.Name), mobileWidth-7)
		b.WriteString(fmt.Sprintf("\n  %s · %d%%\n", name, percent))
		b.WriteString(fmt.Sprintf("  `%s`\n", bar))
		b.WriteString(fmt.Sprintf("  %s / %s  ·  %s", FormatMoney(r.Spent, r.Currency, 0), FormatMoney(r.Available, r.Currency, 0), left))

		totalSpent[r.Currency] = totalSpent[r.Currency].Add(r.Spent)
		totalBudget[r.Currency] = totalBudget[r.Currency].Add(r.Available)
	}

	currencies := make([]string, 0, len(totalSpent))
	for currency := range totalSpent {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	b.WriteString("\n\n" + strings.Repeat("─", mobileWidth))
	for _, currency := range currencies {
		b.WriteString(fmt.Sprintf("\n💵 %s total: %s / %s", currency, FormatMoney(totalSpent[currency], currency, 0), FormatMoney(totalBudget[currency], currency, 0)))
	}
	return b.String()
}

// ─── Lists ────────────────────────────────────────────────

func Accounts(accs []models.Account) string {
	if len(accs) == 0 {
		return "No accounts yet.\n\n`/acc add 💳 Name [currency]`"
	}

	totals := make(map[string]decimal.Decimal)
	cards := make([]Card, 0, len(accs))
	for _, a := range accs {
		totals[a.Currency] = totals[a.Currency].Add(a.Balance)
		cards = append(cards, NewCard(
			fmt.Sprintf("%s %s", a.Emoji, a.Name),
			FormatMoney(a.Balance, a.Currency, 0),
		))
	}

	currencies := make([]string, 0, len(totals))
	for currency := range totals {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	totalLines := make([]string, 0, len(currencies))
	for _, currency := range currencies {
		totalLines = append(totalLines, fmt.Sprintf("*%s total: %s*", currency, FormatMoney(totals[currency], currency, 0)))
	}

	return fmt.Sprintf("💰 *Accounts*\n\n%s\n\n%s", RenderCards(cards), strings.Join(totalLines, "\n"))
}

func Categories(cats []models.Category, groups []models.CategoryGroup) string {
	if len(cats) == 0 {
		return "No categories yet.\n\n`/cat add 🍞 Name`"
	}

	gm := map[int64]models.CategoryGroup{}
	for _, g := range groups {
		gm[g.ID] = g
	}

	// Group cats by group ID
	grouped := map[int64][]models.Category{}
	var ungrouped []models.Category
	groupOrder := []int64{} // nil means ungrouped

	for _, c := range cats {
		if c.GroupID != nil {
			if _, ok := grouped[*c.GroupID]; !ok {
				groupOrder = append(groupOrder, *c.GroupID)
			}
			grouped[*c.GroupID] = append(grouped[*c.GroupID], c)
		} else {
			ungrouped = append(ungrouped, c)
		}
	}

	var b strings.Builder
	b.WriteString("🏷️ *Categories*\n")

	for _, gid := range groupOrder {
		g := gm[gid]
		b.WriteString(fmt.Sprintf("\n%s %s\n", g.Emoji, g.Name))
		for _, c := range grouped[gid] {
			b.WriteString(fmt.Sprintf("  %s %s\n", c.Emoji, c.Name))
		}
	}

	if len(ungrouped) > 0 {
		if len(groupOrder) > 0 {
			b.WriteString("\n📌 Other\n")
		}
		for _, c := range ungrouped {
			b.WriteString(fmt.Sprintf("  %s %s\n", c.Emoji, c.Name))
		}
	}

	return b.String()
}

func Budgets(cats []models.Category, budgets []models.Budget) string {
	if len(budgets) == 0 {
		return "No budgets set yet.\n\nTap a category or use\n`/budget set Name amount [interval]`"
	}

	categories := make(map[int64]models.Category, len(cats))
	for _, category := range cats {
		categories[category.ID] = category
	}

	cards := make([]Card, 0, len(budgets))
	for _, budget := range budgets {
		category, ok := categories[budget.CategoryID]
		if !ok {
			continue
		}
		amount := FormatMoney(budget.Amount, budget.Currency, 0)
		reset := budget.PeriodStart.AddDate(0, budget.IntervalMonths, budget.IntervalDays).Format("Jan 2")
		cards = append(cards, NewCard(
			fmt.Sprintf("%s %s", category.Emoji, category.Name),
			fmt.Sprintf("%s · %s", amount, budget.Description()),
			"Next · "+reset,
		))
	}

	return "🎯 *Budgets*\n\n" + RenderCards(cards)
}

func Groups(groups []models.CategoryGroup) string {
	if len(groups) == 0 {
		return "No groups yet.\n\n`/group add 📁 Name` to create one."
	}
	var b strings.Builder
	b.WriteString("📁 *Groups*\n")
	for _, g := range groups {
		b.WriteString(fmt.Sprintf("\n%s %s", g.Emoji, g.Name))
	}
	return b.String()
}

func Recent(txs []models.Transaction) string {
	if len(txs) == 0 {
		return "No transactions yet!"
	}

	cards := make([]Card, 0, len(txs))
	for _, t := range txs {
		sign := "➖"
		if t.Type == "income" {
			sign = "➕"
		} else if t.Type == "transfer" {
			sign = "↔️"
		}
		amount := FormatMoney(t.Amount, t.Currency, 2)
		header := fmt.Sprintf("%s %s", sign, amount)
		lines := []string{header}
		if t.ReportingAmount != nil && t.ReportingCurrency != "" && t.ReportingCurrency != t.Currency {
			lines = append(lines, "≈ "+FormatMoney(*t.ReportingAmount, t.ReportingCurrency, 2))
		}
		if t.CategoryEmoji != "" {
			lines = append(lines, fmt.Sprintf("%s %s", t.CategoryEmoji, t.CategoryName))
		}
		if t.Type == "transfer" && t.Description != "" {
			lines = append(lines, t.Description)
		}
		lines = append(lines, t.AccountName, t.CreatedAt.Format("Jan 2 · 15:04"))
		cards = append(cards, NewCard(lines...))
	}

	return "📋 *Recent*\n\n" + RenderCards(cards)
}

// ─── Response builders ─────────────────────────────────────

func Created(emoji, name, kind string) string {
	if kind == "" {
		kind = "category"
	}
	return fmt.Sprintf("✅ Created %s: %s *%s*", kind, emoji, name)
}

func Deleted(emoji, name, kind string) string {
	entity := strings.TrimSpace(emoji + " " + name)
	return fmt.Sprintf("✅ Deleted %s: %s", kind, entity)
}

func Error(msg string) string {
	return fmt.Sprintf("❌ %s", msg)
}

func CatName(cats []models.Category, id int64) string {
	for _, c := range cats {
		if c.ID == id {
			return c.Emoji + " " + c.Name
		}
	}
	return "category"
}
