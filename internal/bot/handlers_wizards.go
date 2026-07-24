package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── Expense wizard ────────────────────────────────────────

func (b *Bot) handleAddExpense(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	if len(cats) == 0 {
		return c.Edit("No categories yet! Use `/cat add 🍞 Name`.", mainMenu())
	}
	return c.Edit("*Pick a category:*", categoryKeyboard(cats))
}

func (b *Bot) handleCatPick(c tele.Context) error {
	uid := c.Sender().ID
	defer c.Respond()

	catID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid category"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, uid)

	b.setState(uid, &userState{
		Wizard: ExpenseWizard{CategoryID: catID},
		Step:   StepExpenseAmount,
		MsgID:  c.Message().ID,
		ChatID: c.Chat().ID,
		Prev:   "pick_category",
	})

	text := progressTemplate("➖ *New Expense*", expenseFields(findCatName(cats, catID), "—", "—"))
	return c.Edit(text, cancelBtn())
}

func (b *Bot) receiveAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `42.50`", cancelBtn())
	}

	w := state.expenseW()
	w.Amount = amount
	state.Wizard = w
	state.Step = StepExpenseAccount

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
	if len(accs) == 0 {
		return c.Send("No accounts! Create one with `/acc add 💳 Name`", mainMenu())
	}

	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	text := progressTemplate("➖ *New Expense*", expenseFields(findCatName(cats, w.CategoryID), fmt.Sprintf("€%.2f", amount), "—"))
	b.editTemplate(state, text, accountKeyboard(accs))
	return c.Send("\u200b")
}

// ─── Income wizard ────────────────────────────────────────

func (b *Bot) handleAddIncome(c tele.Context) error {
	b.setState(c.Sender().ID, &userState{
		Wizard: IncomeWizard{},
		Step:   StepIncomeAmount,
		MsgID:  c.Message().ID,
		ChatID: c.Chat().ID,
	})
	text := progressTemplate("➕ *New Income*", incomeFields("—", "—"))
	return c.Edit(text, cancelBtn())
}

func (b *Bot) receiveIncomeAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `1500`", cancelBtn())
	}

	w := state.incomeW()
	w.Amount = amount
	state.Wizard = w
	state.Step = StepIncomeAccount

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
	if len(accs) == 0 {
		return c.Send("No accounts! Create one with `/acc add 💳 Name`", mainMenu())
	}

	text := progressTemplate("➕ *New Income*", incomeFields(fmt.Sprintf("€%.2f", amount), "—"))
	b.editTemplate(state, text, accountKeyboard(accs))
	return c.Send("\u200b")
}

// ─── Move wizard ──────────────────────────────────────────

func (b *Bot) handleMoveBtn(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
	if len(accs) < 2 {
		return c.Edit("Need at least 2 accounts. Use `/acc add 💳 Name`.", mainMenu())
	}

	b.setState(c.Sender().ID, &userState{
		Wizard: MoveWizard{},
		Step:   StepMoveSource,
		MsgID:  c.Message().ID,
		ChatID: c.Chat().ID,
		Prev:   "move_start",
	})
	text := progressTemplate("🔀 *Transfer*", transferFields("—", "—", "—"))
	return c.Edit(text, accountKeyboard(accs))
}

func (b *Bot) receiveMoveAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `500`", cancelBtn())
	}

	w := state.moveW()
	w.Amount = amount
	state.Wizard = w

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	src, _ := b.Store.GetAccount(ctx, w.SourceID)
	dst, _ := b.Store.GetAccount(ctx, w.DestinationID)
	if src == nil || dst == nil {
		b.clearState(c.Sender().ID)
		return c.Send("❌ Account not found. Start over.", mainMenu())
	}

	_, err = b.Store.CreateTransaction(ctx, c.Sender().ID, w.SourceID, nil, "transfer", amount, &w.DestinationID, fmt.Sprintf("→ %s", dst.Name))
	if err != nil {
		b.clearState(c.Sender().ID)
		return c.Send("❌ Error creating transfer.", mainMenu())
	}

	b.clearState(c.Sender().ID)
	text := progressTemplate("🔀 *Transfer*", transferFields(
		src.Emoji+" "+src.Name, dst.Emoji+" "+dst.Name, fmt.Sprintf("€%.2f", amount)))
	return c.Send("✅ Transferred!\n\n"+text, mainMenu())
}

// ─── Budget wizard ─────────────────────────────────────────

func (b *Bot) handleBudgetMenu(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	if len(cats) == 0 {
		return c.Edit("No categories yet. Create some first with `/cat add`.", mainMenu())
	}
	budgets, _ := b.Store.GetBudgets(ctx, c.Sender().ID)
	return c.Edit(msgBudgets(cats, budgets), budgetCategoryKeyboard(cats))
}

func (b *Bot) handleBudgetPick(c tele.Context) error {
	uid := c.Sender().ID
	defer c.Respond()
	catID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)

	b.setState(uid, &userState{
		Wizard: BudgetWizard{CategoryID: catID},
		Step:   StepBudgetAmount,
	})

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, uid)
	for _, cat := range cats {
		if cat.ID == catID {
			return c.Edit(fmt.Sprintf("%s *%s*\n\n_Type the budget amount:_", cat.Emoji, cat.Name), cancelBtn())
		}
	}
	return c.Edit("Type the budget amount:", cancelBtn())
}

func (b *Bot) receiveBudgetAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount < 0 {
		return c.Send("Please enter a valid number, e.g. `5000`", cancelBtn())
	}

	w := state.budgetW()
	w.Amount = amount
	state.Wizard = w
	state.Step = StepBudgetInterval

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	return c.Send(fmt.Sprintf("🎯 Budget for %s: *%.0f*\n\n_Pick an interval:_", findCatName(cats, w.CategoryID), amount), intervalKeyboard())
}

// ─── Account pick → route by variant ──────────────────────

func (b *Bot) handleAccPick(c tele.Context) error {
	uid := c.Sender().ID
	defer c.Respond()

	accID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)
	state := b.stateFor(uid)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	acc, _ := b.Store.GetAccount(ctx, accID)
	if acc == nil {
		return c.Respond(&tele.CallbackResponse{Text: "Account not found"})
	}

	switch w := state.Wizard.(type) {
	case ExpenseWizard:
		_, err := b.Store.CreateTransaction(ctx, uid, acc.ID, &w.CategoryID, "expense", w.Amount, nil, "")
		if err != nil {
			return c.Edit("❌ Error saving transaction.", mainMenu())
		}
		b.clearState(uid)
		cats, _ := b.Store.GetCategories(ctx, uid)
		text := progressTemplate("➖ *New Expense*", expenseFields(findCatName(cats, w.CategoryID), fmt.Sprintf("€%.2f", w.Amount), acc.Emoji+" "+acc.Name))
		return c.Edit("✅ Logged!\n\n"+text, mainMenu())

	case IncomeWizard:
		_, err := b.Store.CreateTransaction(ctx, uid, acc.ID, nil, "income", w.Amount, nil, "")
		if err != nil {
			return c.Edit("❌ Error saving income.", mainMenu())
		}
		b.clearState(uid)
		text := progressTemplate("➕ *New Income*", incomeFields(fmt.Sprintf("€%.2f", w.Amount), acc.Emoji+" "+acc.Name))
		return c.Edit("✅ Income logged!\n\n"+text, mainMenu())

	case MoveWizard:
		switch state.Step {
		case StepMoveSource:
			w.SourceID = acc.ID
			state.Wizard = w
			state.Step = StepMoveTarget
			accs, _ := b.Store.GetAccounts(ctx, uid)
			text := progressTemplate("🔀 *Transfer*", transferFields(acc.Emoji+" "+acc.Name, "—", "—"))
			return c.Edit(text, accountKeyboardExclude(accs, acc.ID))

		case StepMoveTarget:
			w.DestinationID = acc.ID
			state.Wizard = w
			state.Step = StepMoveAmount
			src, _ := b.Store.GetAccount(ctx, w.SourceID)
			srcName := "Unknown"
			if src != nil {
				srcName = src.Emoji + " " + src.Name
			}
			text := progressTemplate("🔀 *Transfer*", transferFields(srcName, acc.Emoji+" "+acc.Name, "—"))
			return c.Edit(text, cancelBtn())
		}
	}

	return c.Respond(&tele.CallbackResponse{Text: "Done!"})
}

// ─── Text dispatcher ──────────────────────────────────────

func (b *Bot) handleText(c tele.Context) error {
	state := b.stateFor(c.Sender().ID)
	if state == nil || !state.IsTextStep() {
		return c.Send("Tap a button or use `/menu`", mainMenu())
	}

	switch state.Step {
	case StepExpenseAmount:
		return b.receiveAmount(c, state)
	case StepIncomeAmount:
		return b.receiveIncomeAmount(c, state)
	case StepMoveAmount:
		return b.receiveMoveAmount(c, state)
	case StepBudgetAmount:
		return b.receiveBudgetAmount(c, state)
	case StepCreateName:
		return b.receiveCreateName(c, state)
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────

func findCatName(cats []models.Category, id int64) string {
	for _, c := range cats {
		if c.ID == id {
			return c.Emoji + " " + c.Name
		}
	}
	return "category"
}
