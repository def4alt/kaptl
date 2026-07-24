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
		return c.Send("No categories yet! Use `/cat add 🍞 Name` to create one.", mainMenu())
	}
	return c.Send("*Pick a category:*", categoryKeyboard(cats))
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
	catName := findCatName(cats, catID)

	b.setState(uid, &userState{
		Step:          StepAwaitExpenseAmount,
		CategoryID:    catID,
		TxType:        "expense",
		TemplateMsgID: c.Message().ID,
		ChatID:        c.Chat().ID,
		PrevStep:      "pick_category",
	})

	text := progressTemplate("➖ *New Expense*", map[string]string{
		"Category": catName, "Amount": "—", "Account": "—",
	})
	return c.Edit(text, cancelBtn())
}

func (b *Bot) receiveAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `42.50`", cancelBtn())
	}
	state.Amount = amount
	state.Step = StepAwaitExpenseAccount

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
	if len(accs) == 0 {
		return c.Send("No accounts! Create one with `/acc add 💳 Name`", mainMenu())
	}

	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	text := progressTemplate("➖ *New Expense*", map[string]string{
		"Category": findCatName(cats, state.CategoryID),
		"Amount":   fmt.Sprintf("€%.2f", amount),
		"Account":  "—",
	})
	b.editTemplate(state, text, accountKeyboard(accs))
	return c.Send("\u200b") // clear input
}

// ─── Income wizard ────────────────────────────────────────

func (b *Bot) handleAddIncome(c tele.Context) error {
	b.setState(c.Sender().ID, &userState{
		Step:          StepAwaitIncomeAmount,
		TxType:        "income",
		TemplateMsgID: c.Message().ID,
		ChatID:        c.Chat().ID,
	})
	text := progressTemplate("➕ *New Income*", map[string]string{"Amount": "—", "Account": "—"})
	return c.Edit(text, cancelBtn())
}

func (b *Bot) receiveIncomeAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `1500`", cancelBtn())
	}
	state.Amount = amount
	state.Step = StepAwaitIncomeAccount

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
	if len(accs) == 0 {
		return c.Send("No accounts! Create one with `/acc add 💳 Name`", mainMenu())
	}

	text := progressTemplate("➕ *New Income*", map[string]string{
		"Amount": fmt.Sprintf("€%.2f", amount), "Account": "—",
	})
	b.editTemplate(state, text, accountKeyboard(accs))
	return c.Send("\u200b")
}

// ─── Move wizard ──────────────────────────────────────────

func (b *Bot) handleMoveBtn(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
	if len(accs) < 2 {
		return c.Send("Need at least 2 accounts. Use `/acc add 💳 Name`.", mainMenu())
	}

	b.setState(c.Sender().ID, &userState{
		Step:          StepAwaitMoveSource,
		TxType:        "transfer",
		TemplateMsgID: c.Message().ID,
		ChatID:        c.Chat().ID,
		PrevStep:      "move_start",
	})
	text := progressTemplate("🔀 *Transfer*", map[string]string{"From": "—", "To": "—", "Amount": "—"})
	return c.Edit(text, accountKeyboard(accs))
}

func (b *Bot) receiveMoveAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `500`", cancelBtn())
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	src, _ := b.Store.GetAccount(ctx, state.AccountID)
	dst, _ := b.Store.GetAccount(ctx, state.TargetAccountID)
	if src == nil || dst == nil {
		b.clearState(c.Sender().ID)
		return c.Send("❌ Account not found. Start over.", mainMenu())
	}

	_, err = b.Store.CreateTransaction(ctx, c.Sender().ID, state.AccountID, nil, "transfer", amount, &state.TargetAccountID, fmt.Sprintf("→ %s", dst.Name))
	if err != nil {
		b.clearState(c.Sender().ID)
		return c.Send("❌ Error creating transfer.", mainMenu())
	}

	b.clearState(c.Sender().ID)
	text := progressTemplate("🔀 *Transfer*", map[string]string{
		"From": src.Emoji + " " + src.Name, "To": dst.Emoji + " " + dst.Name, "Amount": fmt.Sprintf("€%.2f", amount),
	})
	return c.Send("✅ Transferred!\n\n"+text, mainMenu())
}

// ─── Budget wizard ─────────────────────────────────────────

func (b *Bot) handleBudgetMenu(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	if len(cats) == 0 {
		return c.Send("No categories yet. Create some first with `/cat add`.", mainMenu())
	}
	budgets, _ := b.Store.GetBudgets(ctx, c.Sender().ID)
	return c.Send(msgBudgets(cats, budgets), budgetCategoryKeyboard(cats))
}

func (b *Bot) handleBudgetPick(c tele.Context) error {
	uid := c.Sender().ID
	defer c.Respond()
	catID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, uid)

	b.setState(uid, &userState{
		Step:          StepAwaitBudgetAmount,
		EditingBudget: catID,
	})

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
	state.Amount = amount
	state.Step = StepAwaitBudgetInterval

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	catName := findCatName(cats, state.EditingBudget)
	return c.Send(fmt.Sprintf("🎯 Budget for %s: *%.0f*\n\n_Pick an interval:_", catName, amount), intervalKeyboard())
}

// ─── Account pick → completes expense/income/move ──────────

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

	switch state.TxType {
	case "expense":
		_, err := b.Store.CreateTransaction(ctx, uid, acc.ID, &state.CategoryID, "expense", state.Amount, nil, "")
		if err != nil {
			return c.Edit("❌ Error saving transaction.", mainMenu())
		}
		b.clearState(uid)
		cats, _ := b.Store.GetCategories(ctx, uid)
		text := progressTemplate("➖ *New Expense*", map[string]string{
			"Category": findCatName(cats, state.CategoryID),
			"Amount":   fmt.Sprintf("€%.2f", state.Amount),
			"Account":  acc.Emoji + " " + acc.Name,
		})
		return c.Edit("✅ Logged!\n\n"+text, mainMenu())

	case "income":
		_, err := b.Store.CreateTransaction(ctx, uid, acc.ID, nil, "income", state.Amount, nil, "")
		if err != nil {
			return c.Edit("❌ Error saving income.", mainMenu())
		}
		b.clearState(uid)
		text := progressTemplate("➕ *New Income*", map[string]string{
			"Amount": fmt.Sprintf("€%.2f", state.Amount), "Account": acc.Emoji + " " + acc.Name,
		})
		return c.Edit("✅ Income logged!\n\n"+text, mainMenu())

	case "transfer":
		switch state.Step {
		case StepAwaitMoveSource:
			state.Step = StepAwaitMoveTarget
			state.AccountID = acc.ID
			accs, _ := b.Store.GetAccounts(ctx, uid)
			text := progressTemplate("🔀 *Transfer*", map[string]string{
				"From": acc.Emoji + " " + acc.Name, "To": "—", "Amount": "—",
			})
			return c.Edit(text, accountKeyboardExclude(accs, acc.ID))

		case StepAwaitMoveTarget:
			state.Step = StepAwaitMoveAmount
			state.TargetAccountID = acc.ID
			src, _ := b.Store.GetAccount(ctx, state.AccountID)
			srcName := "Unknown"
			if src != nil {
				srcName = src.Emoji + " " + src.Name
			}
			text := progressTemplate("🔀 *Transfer*", map[string]string{
				"From": srcName, "To": acc.Emoji + " " + acc.Name, "Amount": "—",
			})
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
	case StepAwaitExpenseAmount:
		return b.receiveAmount(c, state)
	case StepAwaitIncomeAmount:
		return b.receiveIncomeAmount(c, state)
	case StepAwaitMoveAmount:
		return b.receiveMoveAmount(c, state)
	case StepAwaitBudgetAmount:
		return b.receiveBudgetAmount(c, state)
	case StepAwaitCatName:
		return b.receiveCatName(c, state)
	case StepAwaitAccName:
		return b.receiveAccName(c, state)
	case StepAwaitGroupName:
		return b.receiveGroupName(c, state)
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
