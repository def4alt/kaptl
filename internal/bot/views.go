package bot

import (
	"fmt"

	"github.com/def4alt/kaptl/internal/bot/view"
	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── View wrappers (delegate to view/ package) ─────────────

func msgWelcome() string                                    { return view.Welcome() }
func msgHelp() string                                       { return view.Help() }
func msgSummary(r []models.BudgetRow, rta float64) string   { return view.Summary(r, rta) }
func msgAccounts(a []models.Account) string                 { return view.Accounts(a) }
func msgCategories(c []models.Category, g []models.CategoryGroup) string { return view.Categories(c, g) }
func msgBudgets(c []models.Category, b []models.Budget) string { return view.Budgets(c, b) }
func msgGroups(g []models.CategoryGroup) string             { return view.Groups(g) }
func msgRecent(t []models.Transaction) string               { return view.Recent(t) }
func respondCreated(e, n, k string) string                   { return view.Created(e, n, k) }
func respondError(m string) string                           { return view.Error(m) }
func respondDeleted(e, n, k string) string                   { return view.Deleted(e, n, k) }

func progressTemplate(title string, fields map[string]string) string {
	return view.ProgressTemplate(title, fields)
}

func expenseFields(cat, amt, acc string) map[string]string {
	return map[string]string(view.ExpenseFields(cat, amt, acc))
}
func incomeFields(amt, acc string) map[string]string {
	return map[string]string(view.IncomeFields(amt, acc))
}
func transferFields(from, to, amt string) map[string]string {
	return map[string]string(view.TransferFields(from, to, amt))
}

// ─── Bot-dependent functions ──────────────────────────────

func (b *Bot) editTemplate(state *userState, text string, markup *tele.ReplyMarkup) error {
	if state.MsgID == 0 || state.ChatID == 0 {
		return fmt.Errorf("no template message")
	}
	msg := &tele.Message{ID: state.MsgID, Chat: &tele.Chat{ID: state.ChatID}}
	_, err := b.Tele.Edit(msg, text, markup)
	return err
}

func (b *Bot) editStep(state *userState, title string, fields map[string]string, markup *tele.ReplyMarkup) error {
	return b.editTemplate(state, progressTemplate(title, fields), markup)
}
