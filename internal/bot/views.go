package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── Main messages ─────────────────────────────────────────

func msgWelcome() string {
	return "💰 *Kaptl* — expense tracker\n\nTrack your spending like a pro."
}

func msgHelp() string {
	return `*Commands:*
/start – Main menu
/cat add 🍞 Name – Create category
/cat rm Name – Delete category
/cat list – List categories
/acc add 💳 Name [currency] – Create account
/acc list – List accounts
/budget set Name amount [interval] – Set recurring budget
/group add 📁 Name – Create category group
/group rm Name – Delete group
/move amount from Account to Account – Transfer between accounts

*Quick expense:*
Tap "➕ Expense" → pick category → type amount → pick account → done!

*Currency defaults to EUR.*`
}

// ─── Summary ──────────────────────────────────────────────

func msgSummary(rows []models.BudgetRow, rta float64) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📊 *Budget Summary — %s*\n", time.Now().Format("January 2006")))

	color := "🟢"
	if rta < 0 {
		color = "🔴"
	}
	b.WriteString(fmt.Sprintf("\n💵 *Ready to Assign:* %s €%.0f\n", color, rta))

	totalBudget, totalSpent := 0.0, 0.0
	lastGroup := ""

	for _, r := range rows {
		if r.GroupName != "" && r.GroupName != lastGroup {
			lastGroup = r.GroupName
			b.WriteString(fmt.Sprintf("\n🏷️ *%s*", lastGroup))
		}
		line := fmt.Sprintf("%s *%s*: %.0f / %.0f (%.0f left)", r.Emoji, r.Name, r.Spent, r.Available, r.Remaining)
		if r.Rollover > 0 {
			line += fmt.Sprintf("  _+%.0f rollover_", r.Rollover)
		}
		b.WriteString("\n" + line)
		totalBudget += r.Available
		totalSpent += r.Spent
	}

	b.WriteString(fmt.Sprintf("\n\n💵 Total: *%.0f / %.0f* (%.0f left)", totalSpent, totalBudget, totalBudget-totalSpent))
	return b.String()
}

// ─── Accounts list ────────────────────────────────────────

func msgAccounts(accs []models.Account) string {
	if len(accs) == 0 {
		return "No accounts yet.\n\n`/acc add 💳 Name [currency]`"
	}
	var lines []string
	for _, a := range accs {
		lines = append(lines, fmt.Sprintf("%s *%s*: %.2f %s", a.Emoji, a.Name, a.Balance, a.Currency))
	}
	return "💰 *Accounts*\n\n" + strings.Join(lines, "\n") + "\n\n_Add:_ `/acc add 💳 Name [currency]`"
}

// ─── Categories list ──────────────────────────────────────

func msgCategories(cats []models.Category, groups []models.CategoryGroup) string {
	if len(cats) == 0 {
		return "No categories yet.\n\n_Add:_ `/cat add 🍞 Name`"
	}
	groupMap := map[int64]models.CategoryGroup{}
	for _, g := range groups {
		groupMap[g.ID] = g
	}
	var lines []string
	for _, cat := range cats {
		gs := ""
		if cat.GroupID != nil {
			if g, ok := groupMap[*cat.GroupID]; ok {
				gs = fmt.Sprintf("  _%s %s_", g.Emoji, g.Name)
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s%s", cat.Emoji, cat.Name, gs))
	}
	return "🏷️ *Categories*\n\n" + strings.Join(lines, "\n") + "\n\n`/cat add 🍞 Name`  `/cat rm Name`"
}

// ─── Budgets list ─────────────────────────────────────────

func msgBudgets(cats []models.Category, budgets []models.Budget) string {
	bMap := map[int64]models.Budget{}
	for _, bd := range budgets {
		bMap[bd.CategoryID] = bd
	}
	var lines []string
	for _, cat := range cats {
		if bd, ok := bMap[cat.ID]; ok {
			lines = append(lines, fmt.Sprintf("%s %s: *%.0f* (%s)", cat.Emoji, cat.Name, bd.Amount, bd.Description()))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "_No budgets set_")
	}
	return "🎯 *Budget*\n\n" + strings.Join(lines, "\n") + "\n\n_Tap a category to set budget, or_\n`/budget set Name amount`"
}

// ─── Groups list ─────────────────────────────────────────

func msgGroups(groups []models.CategoryGroup) string {
	if len(groups) == 0 {
		return "No groups yet.\n\n`/group add 📁 Name` to create one."
	}
	var lines []string
	for _, g := range groups {
		lines = append(lines, fmt.Sprintf("%s %s", g.Emoji, g.Name))
	}
	return "📁 *Groups*\n\n" + strings.Join(lines, "\n") + "\n\n`/group add 📁 Name`  `/group rm Name`"
}

// ─── Recent transactions ──────────────────────────────────

func msgRecent(txs []models.Transaction) string {
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

// ─── Progress template for wizards ────────────────────────

func progressTemplate(title string, fields map[string]string) string {
	order := []string{"Category", "Amount", "Account", "From", "To", "Emoji", "Name", "Currency", "Group", "Interval"}
	msg := title + "\n\n"
	for _, key := range order {
		if val, ok := fields[key]; ok {
			msg += fmt.Sprintf("%s: %s\n", key, val)
		}
	}
	return msg
}

func (b *Bot) editTemplate(state *userState, text string, markup *tele.ReplyMarkup) error {
	if state.MsgID == 0 || state.ChatID == 0 {
		return fmt.Errorf("no template message")
	}
	msg := &tele.Message{ID: state.MsgID, Chat: &tele.Chat{ID: state.ChatID}}
	_, err := b.Tele.Edit(msg, text, markup)
	return err
}
