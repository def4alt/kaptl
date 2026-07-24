package bot

import (
	tele "gopkg.in/telebot.v4"
)

// ─── Handler registration ──────────────────────────────────

func RegisterHandlers(tb *tele.Bot, b *Bot) {
	// Core commands
	tb.Handle("/start", b.handleStart)
	tb.Handle("/menu", b.handleStart)
	tb.Handle("/help", b.handleHelp)

	// Resource commands
	tb.Handle("/cat", b.handleCat)
	tb.Handle("/acc", b.handleAcc)
	tb.Handle("/budget", b.handleBudget)
	tb.Handle("/move", b.handleMove)
	tb.Handle("/group", b.handleGroup)

	// Inline buttons
	tb.Handle(&btnAddExpense, b.handleAddExpense)
	tb.Handle(&btnAddIncome, b.handleAddIncome)
	tb.Handle(&btnMove, b.handleMoveBtn)
	tb.Handle(&btnSummary, b.handleSummary)
	tb.Handle(&btnRecent, b.handleRecent)
	tb.Handle(&btnCancel, b.handleCancel)
	tb.Handle(&btnManage, b.handleManageMenu)
	tb.Handle(&btnMgCats, b.handleManageCats)
	tb.Handle(&btnMgAccs, b.handleManageAccs)
	tb.Handle(&btnMgBuds, b.handleManageBuds)
	tb.Handle(&btnMgGrps, b.handleManageGrps)
	tb.Handle(&btnBackMn, b.handleStart)

	// Dynamic callbacks
	tb.Handle("\f"+cbCat, b.handleCatPick)
	tb.Handle("\f"+cbBudget, b.handleBudgetPick)
	tb.Handle("\f"+cbAcc, b.handleAccPick)
	tb.Handle("\f"+cbCancel, b.handleDynamicCancel)
	tb.Handle("\f"+cbBack, b.handleBackBtn)
	tb.Handle("\f"+cbEmoji, b.handleEmojiPick)
	tb.Handle("\f"+cbCurr, b.handleCurrencyPick)
	tb.Handle("\f"+cbIntv, b.handleIntervalPick)
	tb.Handle("\f"+cbGroup, b.handleGroupPick)

	// Text
	tb.Handle(tele.OnText, b.handleText)
}

// ─── Core commands ─────────────────────────────────────────

func (b *Bot) handleStart(c tele.Context) error {
	b.clearState(c.Sender().ID)
	return c.Send(msgWelcome(), mainMenu())
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(msgHelp(), mainMenu())
}
