package bot

// WizardStep is a typed step in the conversational wizard state machine.
type WizardStep int

const (
	StepIdle WizardStep = iota

	// Expense wizard
	StepAwaitExpenseAmount
	StepAwaitExpenseAccount

	// Income wizard
	StepAwaitIncomeAmount
	StepAwaitIncomeAccount

	// Move wizard
	StepAwaitMoveSource
	StepAwaitMoveTarget
	StepAwaitMoveAmount

	// Budget wizard
	StepAwaitBudgetAmount
	StepAwaitBudgetInterval

	// Creation wizards
	StepAwaitCatEmoji
	StepAwaitCatName
	StepAwaitCatGroup
	StepAwaitAccEmoji
	StepAwaitAccName
	StepAwaitAccCurrency
	StepAwaitGroupEmoji
	StepAwaitGroupName
)

// String returns the step name for debugging.
func (s WizardStep) String() string {
	names := map[WizardStep]string{
		StepIdle:                "idle",
		StepAwaitExpenseAmount:  "expense_amount",
		StepAwaitExpenseAccount: "expense_account",
		StepAwaitIncomeAmount:   "income_amount",
		StepAwaitIncomeAccount:  "income_account",
		StepAwaitMoveSource:     "move_source",
		StepAwaitMoveTarget:     "move_target",
		StepAwaitMoveAmount:     "move_amount",
		StepAwaitBudgetAmount:   "budget_amount",
		StepAwaitBudgetInterval: "budget_interval",
		StepAwaitCatEmoji:       "cat_emoji",
		StepAwaitCatName:        "cat_name",
		StepAwaitCatGroup:       "cat_group",
		StepAwaitAccEmoji:       "acc_emoji",
		StepAwaitAccName:        "acc_name",
		StepAwaitAccCurrency:    "acc_currency",
		StepAwaitGroupEmoji:     "group_emoji",
		StepAwaitGroupName:      "group_name",
	}
	if n, ok := names[s]; ok {
		return n
	}
	return "unknown"
}

// userState holds partial data during multi-step wizards.
type userState struct {
	Step            WizardStep
	CategoryID      int64
	AccountID       int64
	TargetAccountID int64
	Amount          float64
	Emoji           string // temp storage for emoji during creation wizards
	Name            string // temp storage for name during creation wizards
	TxType          string // "expense", "income", "transfer"
	EditingBudget   int64
	TemplateMsgID   int
	ChatID          int64
	PrevStep        string // for back navigation: "pick_category", "move_start", etc.
}

// IsWizard returns true if the user is in a multi-step flow.
func (s *userState) IsWizard() bool {
	return s != nil && s.Step != StepIdle
}

// IsTextStep returns true if the current step expects text input.
func (s *userState) IsTextStep() bool {
	if s == nil {
		return false
	}
	switch s.Step {
	case StepAwaitExpenseAmount, StepAwaitIncomeAmount, StepAwaitMoveAmount,
		StepAwaitBudgetAmount, StepAwaitCatName, StepAwaitAccName, StepAwaitGroupName:
		return true
	}
	return false
}
