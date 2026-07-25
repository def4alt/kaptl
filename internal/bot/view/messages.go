package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/def4alt/kaptl/internal/models"
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
/budget set Name amount [interval] – Set recurring budget
/group add 📁 Name – Create category group
/group rm Name – Delete group
/move amount from Account to Account – Transfer

*Quick expense:*
Tap "➖ Expense" → pick category → type amount → pick account → done!

*Currency defaults to EUR.*`
}

// ─── Summary ──────────────────────────────────────────────

func Summary(rows []models.BudgetRow, rta float64) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📊 *%s*\n", time.Now().Format("January 2006")))

	color := "🟢"
	if rta < 0 {
		color = "🔴"
	}
	b.WriteString(fmt.Sprintf("💵 Ready to Assign: %s %s\n", color, formatAmount(rta)))

	totalSpent, totalBudget := 0.0, 0.0
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

		// Progress bar
		pct := 0.0
		if r.Available > 0 {
			pct = r.Spent / r.Available
			if pct > 1 {
				pct = 1
			}
		}
		filled := int(pct * barWidth)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

		// Left status
		left := ""
		if r.Remaining > 0 {
			left = fmt.Sprintf("+%s left", formatAmount(r.Remaining))
		} else if r.Remaining < 0 {
			left = fmt.Sprintf("-%s over", formatAmount(-r.Remaining))
		} else {
			left = "€0 left"
		}

		name := truncateDisplay(fmt.Sprintf("%s %s", r.Emoji, r.Name), mobileWidth-7)
		b.WriteString(fmt.Sprintf("\n  %s · %d%%\n", name, int(pct*100)))
		b.WriteString(fmt.Sprintf("  `%s`\n", bar))
		b.WriteString(fmt.Sprintf("  %s / %s  ·  %s", formatAmount(r.Spent), formatAmount(r.Available), left))

		totalSpent += r.Spent
		totalBudget += r.Available
	}

	b.WriteString(fmt.Sprintf("\n\n%s\n💵 Total: %s / %s", strings.Repeat("─", mobileWidth), formatAmount(totalSpent), formatAmount(totalBudget)))
	return b.String()
}

// ─── Lists ────────────────────────────────────────────────

func Accounts(accs []models.Account) string {
	if len(accs) == 0 {
		return "No accounts yet.\n\n`/acc add 💳 Name [currency]`"
	}

	var total float64
	cards := make([]Card, 0, len(accs))
	for _, a := range accs {
		total += a.Balance
		cards = append(cards, NewCard(
			fmt.Sprintf("%s %s", a.Emoji, a.Name),
			formatAmount(a.Balance),
		))
	}

	return fmt.Sprintf("💰 *Accounts*\n\n%s\n\n*Total: %s*", RenderCards(cards), formatAmount(total))
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
		return "No budgets set yet.\n\nTap a category or use\n`/budget set Name amount`"
	}

	bm := map[int64]models.Budget{}
	for _, bd := range budgets {
		bm[bd.CategoryID] = bd
	}

	cards := make([]Card, 0, len(budgets))
	for _, c := range cats {
		bd, ok := bm[c.ID]
		if !ok {
			continue
		}
		amount := formatAmount(bd.Amount)
		interval := bd.Description()
		reset := bd.PeriodStart.AddDate(0, bd.IntervalMonths, bd.IntervalDays).Format("Jan 2")
		cards = append(cards, NewCard(
			fmt.Sprintf("%s %s", c.Emoji, c.Name),
			fmt.Sprintf("%s · %s", amount, interval),
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
		amount := formatAmount(t.Amount)
		header := fmt.Sprintf("%s %s", sign, amount)
		lines := []string{header}
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
