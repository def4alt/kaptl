package bot

import (
	"fmt"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v3"
)

// ─── Inline button data (unique identifiers for callbacks) ──

var (
	btnAddExpense  = tele.Btn{Unique: "add_expense", Text: "➕ Expense"}
	btnAddIncome   = tele.Btn{Unique: "add_income", Text: "💵 Income"}
	btnSummary     = tele.Btn{Unique: "summary", Text: "📊 Summary"}
	btnAccounts    = tele.Btn{Unique: "accounts", Text: "💰 Accounts"}
	btnCategories  = tele.Btn{Unique: "categories", Text: "🏷️ Categories"}
	btnBudgets     = tele.Btn{Unique: "budgets", Text: "🎯 Budgets"}
	btnRecent      = tele.Btn{Unique: "recent", Text: "📋 Recent"}
	btnCancel      = tele.Btn{Unique: "cancel", Text: "❌ Cancel"}
)

// ─── Category selection keyboard ──────────────────────────

func categoryKeyboard(cats []models.Category, prefix string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, cat := range cats {
		data := fmt.Sprintf("%scat_%d", prefix, cat.ID)
		btn := menu.Data(fmt.Sprintf("%s %s", cat.Emoji, cat.Name), data)
		rows = append(rows, menu.Row(btn))
	}

	if prefix == "budget_" {
		rows = append(rows, menu.Row(menu.Data("❌ Cancel", "cancel")))
	}

	menu.Inline(rows...)
	return menu
}

// ─── Budget category selection ────────────────────────────

func budgetCategoryKeyboard(cats []models.Category) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, cat := range cats {
		data := fmt.Sprintf("budget_%d", cat.ID)
		btn := menu.Data(fmt.Sprintf("%s %s", cat.Emoji, cat.Name), data)
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("❌ Cancel", "cancel")))
	menu.Inline(rows...)
	return menu
}

// ─── Account selection keyboard ───────────────────────────

func accountKeyboard(accs []models.Account) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for _, a := range accs {
		emoji := "💳"
		switch a.Type {
		case "cash":
			emoji = "💵"
		case "savings":
			emoji = "🏦"
		default:
			emoji = "🏛️"
		}
		data := fmt.Sprintf("acc_%d", a.ID)
		btn := menu.Data(fmt.Sprintf("%s %s", emoji, a.Name), data)
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(menu.Data("❌ Cancel", "cancel")))
	menu.Inline(rows...)
	return menu
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

// ─── Cancel button only ───────────────────────────────────

func cancelBtn() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(btnCancel))
	return menu
}
