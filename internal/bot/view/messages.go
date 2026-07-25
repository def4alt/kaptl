package view

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

const barWidth = 14
const cardWidth = 14

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

		// Left status
		left := ""
		if r.Remaining > 0 {
			left = fmt.Sprintf("+%s left", formatAmount(r.Remaining))
		} else if r.Remaining < 0 {
			left = fmt.Sprintf("-%s over", formatAmount(-r.Remaining))
		} else {
			left = "€0 left"
		}

		name := truncate(fmt.Sprintf("%s %s", r.Emoji, r.Name), 24)
		b.WriteString(fmt.Sprintf("\n  %s\n", name))
		b.WriteString(fmt.Sprintf("     %s   %d%%\n", bar, int(pct*100)))
		b.WriteString(fmt.Sprintf("     %s / %s  ·  %s", formatAmount(r.Spent), formatAmount(r.Available), left))

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

	w := cardWidth

	var total float64
	var b strings.Builder
	b.WriteString("💰 *Accounts*\n")

	for _, a := range accs {
		total += a.Balance
		name := truncate(fmt.Sprintf("%s %s", a.Emoji, a.Name), w-1)
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
	neg := v < 0
	if neg { v = -v }
	intPart := fmt.Sprintf("%.0f", v)
	var result []byte
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	if neg {
		return "-€" + string(result)
	}
	return "€" + string(result)
}

// runeLen returns the visual character count (runes, not bytes).
func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func padTo(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func max(a, b int) int {
	if a > b { return a }
	return b
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
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

	w := cardWidth
	var b strings.Builder
	b.WriteString("🎯 *Budgets*\n")

	for _, c := range cats {
		bd, ok := bm[c.ID]
		if !ok {
			continue
		}
		amount := formatAmount(bd.Amount)
		interval := bd.Description()
		reset := bd.PeriodStart.AddDate(0, bd.IntervalMonths, bd.IntervalDays).Format("Jan 2")

		b.WriteString(fmt.Sprintf("\n┌%s┐\n", strings.Repeat("─", w+2)))
		b.WriteString(fmt.Sprintf("│ %s %s%s │\n", c.Emoji, c.Name, strings.Repeat(" ", max(0, w-runeLen(c.Emoji)-runeLen(c.Name)-1))))
		b.WriteString(fmt.Sprintf("│   %s %s%s │\n", amount, interval, strings.Repeat(" ", max(0, w-runeLen(amount)-runeLen(interval)-3))))
		b.WriteString(fmt.Sprintf("│   Resets: %s%s │\n", reset, strings.Repeat(" ", max(0, w-runeLen(reset)-10))))
		b.WriteString(fmt.Sprintf("└%s┘", strings.Repeat("─", w+2)))
	}

	return b.String()
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

	w := cardWidth
	var b strings.Builder
	b.WriteString("📋 *Recent*\n")

	for _, t := range txs {
		sign := "➖"
		if t.Type == "income" {
			sign = "➕"
		} else if t.Type == "transfer" {
			sign = "↔️"
		}
		amount := formatAmount(t.Amount)

		b.WriteString(fmt.Sprintf("\n┌%s┐\n", strings.Repeat("─", w+2)))
		header := fmt.Sprintf("%s %s", sign, amount)
		b.WriteString(fmt.Sprintf("│ %s%s │\n", header, strings.Repeat(" ", max(0, w-runeLen(header)))))

		if t.CategoryEmoji != "" {
			cat := fmt.Sprintf("%s %s", t.CategoryEmoji, t.CategoryName)
			b.WriteString(fmt.Sprintf("│ %s%s │\n", cat, strings.Repeat(" ", max(0, w-runeLen(cat)))))
		}
		if t.Type == "transfer" && t.Description != "" {
			b.WriteString(fmt.Sprintf("│ %s%s │\n", t.Description, strings.Repeat(" ", max(0, w-runeLen(t.Description)))))
		}

		footer := fmt.Sprintf("%s · %s", t.AccountName, t.CreatedAt.Format("Jan 2 15:04"))
		b.WriteString(fmt.Sprintf("│ %s%s │\n", footer, strings.Repeat(" ", max(0, w-runeLen(footer)))))
		b.WriteString(fmt.Sprintf("└%s┘", strings.Repeat("─", w+2)))
	}

	return b.String()
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
