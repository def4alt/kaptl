package bot

import (
	"github.com/def4alt/kaptl/internal/bot/view"
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
	tb.Handle(&btnAddExpense, withCallbackAck(b.handleAddExpense))
	tb.Handle(&btnAddIncome, withCallbackAck(b.handleAddIncome))
	tb.Handle(&btnMove, withCallbackAck(b.handleMoveBtn))
	tb.Handle(&btnSummary, withCallbackAck(b.handleSummary))
	tb.Handle(&btnRecent, withCallbackAck(b.handleRecent))
	tb.Handle(&btnManage, withCallbackAck(b.handleManageMenu))
	tb.Handle(&btnMgCats, withCallbackAck(b.handleManageCats))
	tb.Handle(&btnMgAccs, withCallbackAck(b.handleManageAccs))
	tb.Handle(&btnMgBuds, withCallbackAck(b.handleManageBuds))
	tb.Handle(&btnMgGrps, withCallbackAck(b.handleManageGrps))
	tb.Handle(&btnBackMain, withCallbackAck(b.handleStart))
	tb.Handle(&btnBackManage, withCallbackAck(b.handleManageMenu))
	// Dynamic callbacks
	tb.Handle("\f"+cbCat, b.handleCatPick)
	tb.Handle("\f"+cbBudget, b.handleBudgetPick)
	tb.Handle("\f"+cbAcc, b.handleAccPick)
	tb.Handle("\f"+cbCancel, b.handleDynamicCancel)
	tb.Handle("\f"+cbBack, b.handleBackBtn)
	tb.Handle("\f"+cbEmoji, b.handleEmojiPick)
	tb.Handle("\f"+cbCurr, b.handleCurrencyPick)
	tb.Handle("\f"+cbBudgetCurr, b.handleBudgetCurrencyPick)
	tb.Handle("\f"+cbIntv, b.handleIntervalPick)
	tb.Handle("\f"+cbGroup, b.handleGroupPick)

	// Text
	tb.Handle(tele.OnText, b.handleText)
}

func withCallbackAck(handler tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		defer c.Respond()
		return handler(c)
	}
}

// ─── Core commands ─────────────────────────────────────────

func (b *Bot) handleStart(c tele.Context) error {
	b.clearState(c.Sender().ID)
	if c.Callback() != nil {
		return c.Edit(view.Welcome(), mainMenu())
	}
	return c.Send(view.Welcome(), mainMenu())
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(view.Help(), mainMenu())
}
