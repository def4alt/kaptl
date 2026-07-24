package bot

import (
	"strconv"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── Static inline buttons ─────────────────────────────────

var (
	btnAddExpense = tele.Btn{Unique: "add_expense", Text: "➖ Expense"}
	btnAddIncome  = tele.Btn{Unique: "add_income", Text: "➕ Income"}
	btnMove       = tele.Btn{Unique: "move", Text: "🔀 Move"}
	btnSummary    = tele.Btn{Unique: "summary", Text: "📊 Summary"}
	btnRecent     = tele.Btn{Unique: "recent", Text: "📋 Recent"}
	btnManage     = tele.Btn{Unique: "manage", Text: "⚙️ Manage"}
	btnCancel     = tele.Btn{Unique: "cancel", Text: "❌ Cancel"}

	// Manage submenu
	btnMgCats = tele.Btn{Unique: "mg_cats", Text: "🏷️ Categories"}
	btnMgAccs = tele.Btn{Unique: "mg_accs", Text: "💰 Accounts"}
	btnMgBuds = tele.Btn{Unique: "mg_buds", Text: "🎯 Budgets"}
	btnMgGrps = tele.Btn{Unique: "mg_grps", Text: "📁 Groups"}
	btnBackMn = tele.Btn{Unique: "mg_back", Text: "◀ Back"}
)

// ─── Callback data prefixes ────────────────────────────────

const (
	cbCat    = "cat"
	cbBudget = "budget"
	cbAcc    = "acc"
	cbCancel = "cancel"
	cbBack   = "back"
	cbEmoji  = "emoji"
	cbCurr   = "curr"
	cbIntv   = "intv"
	cbGroup  = "group"
)

// ─── Category selection ────────────────────────────────────

func categoryKeyboard(cats []models.Category) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, cat := range cats {
		rows = append(rows, menu.Row(menu.Data(cat.Emoji+" "+cat.Name, cbCat, strconv.FormatInt(cat.ID, 10))))
	}
	rows = append(rows, menu.Row(menu.Data("❌ Cancel", cbCancel)))
	menu.Inline(rows...)
	return menu
}

func budgetCategoryKeyboard(cats []models.Category) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, cat := range cats {
		rows = append(rows, menu.Row(menu.Data(cat.Emoji+" "+cat.Name, cbBudget, strconv.FormatInt(cat.ID, 10))))
	}
	rows = append(rows, menu.Row(menu.Data("❌ Cancel", cbCancel)))
	menu.Inline(rows...)
	return menu
}

func accountKeyboard(accs []models.Account) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, a := range accs {
		rows = append(rows, menu.Row(menu.Data(a.Emoji+" "+a.Name, cbAcc, strconv.FormatInt(a.ID, 10))))
	}
	rows = append(rows, menu.Row(menu.Data("◀ Back", cbBack), menu.Data("❌ Cancel", cbCancel)))
	menu.Inline(rows...)
	return menu
}

func accountKeyboardExclude(accs []models.Account, excludeID int64) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, a := range accs {
		if a.ID == excludeID {
			continue
		}
		rows = append(rows, menu.Row(menu.Data(a.Emoji+" "+a.Name, cbAcc, strconv.FormatInt(a.ID, 10))))
	}
	rows = append(rows, menu.Row(menu.Data("◀ Back", cbBack), menu.Data("❌ Cancel", cbCancel)))
	menu.Inline(rows...)
	return menu
}

// ─── Emoji picker ──────────────────────────────────────────

var quickEmojis = []string{"🍞", "🏠", "🚆", "🛒", "💳", "💵", "💰", "🎮", "✈️", "🎁", "🛍️", "🍽️", "💻", "📱", "💪", "🧠", "🛡️", "🎓", "🏦", "📁"}

func emojiKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0)
	row := make([]tele.Btn, 0, 5)
	for _, e := range quickEmojis {
		row = append(row, menu.Data(e, cbEmoji, e))
		if len(row) == 5 {
			rows = append(rows, menu.Row(row...))
			row = make([]tele.Btn, 0, 5)
		}
	}
	if len(row) > 0 {
		rows = append(rows, menu.Row(row...))
	}
	rows = append(rows, menu.Row(menu.Data("❌ Cancel", cbCancel)))
	menu.Inline(rows...)
	return menu
}

// ─── Currency picker ───────────────────────────────────────

func currencyKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("EUR", cbCurr, "EUR"), menu.Data("USD", cbCurr, "USD"), menu.Data("UAH", cbCurr, "UAH")),
		menu.Row(menu.Data("PLN", cbCurr, "PLN"), menu.Data("GBP", cbCurr, "GBP")),
		menu.Row(menu.Data("❌ Cancel", cbCancel)),
	)
	return menu
}

// ─── Interval picker ───────────────────────────────────────

func intervalKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("Weekly", cbIntv, "weekly"), menu.Data("Monthly", cbIntv, "monthly")),
		menu.Row(menu.Data("Biweekly", cbIntv, "biweekly"), menu.Data("Quarterly", cbIntv, "quarterly")),
		menu.Row(menu.Data("❌ Cancel", cbCancel)),
	)
	return menu
}

// ─── Group picker ──────────────────────────────────────────

func groupPickerKeyboard(groups []models.CategoryGroup) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	rows := []tele.Row{menu.Row(menu.Data("📌 No group", cbGroup, "0"))}
	for _, g := range groups {
		rows = append(rows, menu.Row(menu.Data(g.Emoji+" "+g.Name, cbGroup, strconv.FormatInt(g.ID, 10))))
	}
	rows = append(rows, menu.Row(menu.Data("❌ Cancel", cbCancel)))
	menu.Inline(rows...)
	return menu
}

// ─── Menus ─────────────────────────────────────────────────

func mainMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(btnAddExpense, btnAddIncome, btnMove),
		menu.Row(btnSummary, btnRecent),
		menu.Row(btnManage),
	)
	return menu
}

func manageMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(btnMgCats, btnMgAccs),
		menu.Row(btnMgBuds, btnMgGrps),
		menu.Row(btnBackMn),
	)
	return menu
}

func cancelBtn() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("❌ Cancel", cbCancel)))
	return menu
}
