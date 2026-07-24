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

const barWidth = 20

func Summary(rows []models.BudgetRow, rta float64) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📊 *%s*\n", time.Now().Format("January 2006")))

	color := "🟢"
	if rta < 0 {
		color = "🔴"
	}
	b.WriteString(fmt.Sprintf("💵 Ready to Assign: %s €%.0f\n", color, rta))

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

		// Left amount
		left := ""
		if r.Remaining > 0 {
			left = fmt.Sprintf("+%s left", formatAmount(r.Remaining))
		} else if r.Remaining < 0 {
			left = fmt.Sprintf("-%s over", formatAmount(-r.Remaining))
		} else {
			left = "€0 left"
		}

		b.WriteString(fmt.Sprintf("\n  %s %s\n", r.Emoji, r.Name))
		b.WriteString(fmt.Sprintf("     %s   %s\n", bar, left))
		b.WriteString(fmt.Sprintf("     %s / %s", formatAmount(r.Spent), formatAmount(r.Available)))

		totalSpent += r.Spent
		totalBudget += r.Available
	}

	b.WriteString(fmt.Sprintf("\n\n────────────────\n💵 Total: %s / %s", formatAmount(totalSpent), formatAmount(totalBudget)))
	return b.String()
}

// ─── Lists ────────────────────────────────────────────────

func Accounts(accs []models.Account) string {
	if len(accs) == 0 {
		return "No accounts yet.\n\n`/acc add 💳 Name [currency]`"
	}

	const w = 20

	var total float64
	var b strings.Builder
	b.WriteString("💰 *Accounts*\n")

	for _, a := range accs {
		total += a.Balance
		name := fmt.Sprintf("%s %s", a.Emoji, a.Name)
		amount := formatAmount(a.Balance)

		b.WriteString(fmt.Sprintf("\n┌%s┐\n", strings.Repeat("─", w+2)))
		sp1 := w - len(name)
		if sp1 < 0 { sp1 = 0 }
		b.WriteString(fmt.Sprintf("│ %s%s │\n", name, strings.Repeat(" ", sp1)))
		sp2 := w - len(amount)
		if sp2 < 0 { sp2 = 0 }
		b.WriteString(fmt.Sprintf("│ %s%s │\n", amount, strings.Repeat(" ", sp2)))
		b.WriteString(fmt.Sprintf("└%s┘", strings.Repeat("─", w+2)))
	}

	b.WriteString(fmt.Sprintf("\n\n         Total: %s", formatAmount(total)))
	return b.String()
}

func formatAmount(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("€%'.f", v)
	}
	return fmt.Sprintf("-€%'.f", -v)
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func Categories(cats []models.Category, groups []models.CategoryGroup) string {
	if len(cats) == 0 {
		return "No categories yet.\n\n_Add:_ `/cat add 🍞 Name`"
	}
	gm := map[int64]models.CategoryGroup{}
	for _, g := range groups {
		gm[g.ID] = g
	}
	var lines []string
	for _, c := range cats {
		gs := ""
		if c.GroupID != nil {
			if g, ok := gm[*c.GroupID]; ok {
				gs = fmt.Sprintf("  _%s %s_", g.Emoji, g.Name)
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s%s", c.Emoji, c.Name, gs))
	}
	return "🏷️ *Categories*\n\n" + strings.Join(lines, "\n") + "\n\n`/cat add 🍞 Name`  `/cat rm Name`"
}

func Budgets(cats []models.Category, budgets []models.Budget) string {
	bm := map[int64]models.Budget{}
	for _, bd := range budgets {
		bm[bd.CategoryID] = bd
	}
	var lines []string
	for _, c := range cats {
		if bd, ok := bm[c.ID]; ok {
			lines = append(lines, fmt.Sprintf("%s %s: *%.0f* (%s)", c.Emoji, c.Name, bd.Amount, bd.Description()))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "_No budgets set_")
	}
	return "🎯 *Budget*\n\n" + strings.Join(lines, "\n") + "\n\n_Tap a category to set budget, or_\n`/budget set Name amount`"
}

func Groups(groups []models.CategoryGroup) string {
	if len(groups) == 0 {
		return "No groups yet.\n\n`/group add 📁 Name` to create one."
	}
	var lines []string
	for _, g := range groups {
		lines = append(lines, fmt.Sprintf("%s %s", g.Emoji, g.Name))
	}
	return "📁 *Groups*\n\n" + strings.Join(lines, "\n") + "\n\n`/group add 📁 Name`  `/group rm Name`"
}

func Recent(txs []models.Transaction) string {
	if len(txs) == 0 {
		return "No transactions yet!"
	}
	var lines []string
	for _, t := range txs {
		sign := "➖"
		if t.Type == "income" {
			sign = "➕"
		} else if t.Type == "transfer" {
			sign = "↔️"
		}
		line := fmt.Sprintf("%s *%.2f* — %s", sign, t.Amount, t.AccountName)
		if t.Type == "transfer" && t.Description != "" {
			line += " " + t.Description
		}
		if t.CategoryEmoji != "" && t.CategoryName != "" {
			line += fmt.Sprintf(" | %s %s", t.CategoryEmoji, t.CategoryName)
		}
		line += fmt.Sprintf("\n  _%s_", t.CreatedAt.Format("Jan 2 15:04"))
		lines = append(lines, line)
	}
	return "📋 *Recent Transactions*\n\n" + strings.Join(lines, "\n")
}

// ─── Response builders ─────────────────────────────────────

func Created(emoji, name, kind string) string {
	return fmt.Sprintf("✅ Created %s: %s *%s*", kind, emoji, name)
}

func Deleted(emoji, name, kind string) string {
	return fmt.Sprintf("✅ Deleted %s: %s %s", kind, emoji, name)
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
