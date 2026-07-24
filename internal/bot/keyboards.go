package bot

import (
	"fmt"
	"strconv"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── Static inline buttons (handled via Handle(&btn, ...)) ──

var (
	btnAddExpense = tele.Btn{Unique: "add_expense", Text: "➖ Expense"}
	btnAddIncome  = tele.Btn{Unique: "add_income", Text: "➕ Income"}
	btnMove       = tele.Btn{Unique: "move", Text: "🔀 Move"}
	btnSummary    = tele.Btn{Unique: "summary", Text: "📊 Summary"}
	btnAccounts   = tele.Btn{Unique: "accounts", Text: "💰 Accounts"}
	btnCategories = tele.Btn{Unique: "categories", Text: "🏷️ Categories"}
	btnBudgets    = tele.Btn{Unique: "budgets", Text: "🎯 Budgets"}
	btnRecent     = tele.Btn{Unique: "recent", Text: "📋 Recent"}
	btnCancel     = tele.Btn{Unique: "cancel", Text: "❌ Cancel"}
)

// ─── Callback data prefixes (telebot routes as \f+prefix) ──

const (
	cbCat    = "cat"    // category pick
	cbBudget = "budget" // budget pick
	cbAcc    = "acc"    // account pick
	cbCancel = "cancel" // cancel wizard
)

// ─── Category selection keyboard ──────────────────────────

func categoryKeyboard(cats []models.Category) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, cat := range cats {
		btn := menu.Data(
			fmt.Sprintf("%s %s", cat.Emoji, cat.Name),
			cbCat, strconv.FormatInt(cat.ID, 10),
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
			cbBudget, strconv.FormatInt(cat.ID, 10),
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
			cbAcc, strconv.FormatInt(a.ID, 10),
		)
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(cancelDataBtn(menu)))
	menu.Inline(rows...)
	return menu
}

// ─── Account selection keyboard (excluding one account) ────

func accountKeyboardExclude(accs []models.Account, excludeID int64) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, a := range accs {
		if a.ID == excludeID {
			continue
		}
		btn := menu.Data(
			fmt.Sprintf("%s %s", a.Emoji, a.Name),
			cbAcc, strconv.FormatInt(a.ID, 10),
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
		menu.Row(btnMove),
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
