package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── Static inline buttons (handled via Handle(&btn, ...)) ──

var (
	btnAddExpense = tele.Btn{Unique: "add_expense", Text: "➕ Expense"}
	btnAddIncome  = tele.Btn{Unique: "add_income", Text: "💵 Income"}
	btnSummary    = tele.Btn{Unique: "summary", Text: "📊 Summary"}
	btnAccounts   = tele.Btn{Unique: "accounts", Text: "💰 Accounts"}
	btnCategories = tele.Btn{Unique: "categories", Text: "🏷️ Categories"}
	btnBudgets    = tele.Btn{Unique: "budgets", Text: "🎯 Budgets"}
	btnRecent     = tele.Btn{Unique: "recent", Text: "📋 Recent"}
	btnCancel     = tele.Btn{Unique: "cancel", Text: "❌ Cancel"}
)

// ─── Callback data prefixes (telebot pipes them: "cat|5") ──

const (
	cbCat    = "cat"    // category pick: cat|<id>
	cbBudget = "budget" // budget pick: budget|<id>
	cbAcc    = "acc"    // account pick: acc|<id>
	cbCancel = "cancel" // cancel wizard
)

// parseCallback splits "prefix|value" into two parts.
func parseCallback(data string) (prefix, value string) {
	parts := strings.SplitN(data, "|", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return data, ""
}

// callbackData builds a telebot callback data string.
func callbackData(prefix string, id int64) string {
	return prefix + "|" + strconv.FormatInt(id, 10)
}

// ─── Category selection keyboard ──────────────────────────

func categoryKeyboard(cats []models.Category) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, cat := range cats {
		btn := menu.Data(
			fmt.Sprintf("%s %s", cat.Emoji, cat.Name),
			callbackData(cbCat, cat.ID),
		)
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(cancelDataBtn(menu)))
	menu.Inline(rows...)
	return menu
}

// ─── Budget category selection ────────────────────────────

func budgetCategoryKeyboard(cats []models.Category) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, cat := range cats {
		btn := menu.Data(
			fmt.Sprintf("%s %s", cat.Emoji, cat.Name),
			callbackData(cbBudget, cat.ID),
		)
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(cancelDataBtn(menu)))
	menu.Inline(rows...)
	return menu
}

// ─── Account selection keyboard ───────────────────────────

func accountKeyboard(accs []models.Account) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, a := range accs {
		btn := menu.Data(
			fmt.Sprintf("%s %s", a.Emoji, a.Name),
			callbackData(cbAcc, a.ID),
		)
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(cancelDataBtn(menu)))
	menu.Inline(rows...)
	return menu
}

// ─── Cancel button (as data, not static Btn) ──────────────

func cancelDataBtn(menu *tele.ReplyMarkup) tele.Btn {
	return menu.Data("❌ Cancel", cbCancel)
}

// ─── Main menu ────────────────────────────────────────────

func mainMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(btnAddExpense, btnAddIncome),
		menu.Row(btnSummary, btnBudgets),
		menu.Row(btnAccounts, btnCategories),
		menu.Row(btnRecent),
	)
	return menu
}

// ─── Cancel button only (reuse static btn) ────────────────

func cancelBtn() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(cancelDataBtn(menu)))
	return menu
}
