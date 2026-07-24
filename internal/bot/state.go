package bot

// ─── Wizard variants (discriminated union) ─────────────────

// Wizard is a sealed interface — only the types below implement it.
type Wizard interface {
	isWizard()
	// Type returns a discriminator string for routing callbacks.
	Type() string
}

// ExpenseWizard is the state for the expense logging flow.
type ExpenseWizard struct {
	CategoryID int64
	Amount     float64
}

// IncomeWizard is the state for the income logging flow.
type IncomeWizard struct {
	Amount float64
}

// MoveWizard is the state for the account transfer flow.
type MoveWizard struct {
	SourceID      int64
	DestinationID int64
	Amount        float64
}

// BudgetWizard is the state for the budget setting flow.
type BudgetWizard struct {
	CategoryID int64
	Amount     float64
}

// CreationWizard is the state for interactive entity creation.
type CreationWizard struct {
	Kind     string // "cat", "acc", "group"
	Emoji    string
	Name     string
	CatGroup *int64 // for category creation group picker
}

// Seal the interface.
func (ExpenseWizard) isWizard()  {}
func (IncomeWizard) isWizard()   {}
func (MoveWizard) isWizard()     {}
func (BudgetWizard) isWizard()   {}
func (CreationWizard) isWizard() {}

// Type returns the discriminator.
func (ExpenseWizard) Type() string  { return "expense" }
func (IncomeWizard) Type() string   { return "income" }
func (MoveWizard) Type() string     { return "transfer" }
func (BudgetWizard) Type() string   { return "budget" }
func (CreationWizard) Type() string { return "creation" }

// ─── Wizard steps ──────────────────────────────────────────

type WizardStep int

const (
	StepIdle WizardStep = iota

	// Expense
	StepExpenseAmount
	StepExpenseAccount

	// Income
	StepIncomeAmount
	StepIncomeAccount

	// Move
	StepMoveSource
	StepMoveTarget
	StepMoveAmount

	// Budget
	StepBudgetAmount
	StepBudgetInterval

	// Creation
	StepCreateEmoji
	StepCreateName
	StepCreateGroup
	StepCreateCurrency
)

func (s WizardStep) String() string {
	names := map[WizardStep]string{
		StepIdle:            "idle",
		StepExpenseAmount:   "expense_amount",
		StepExpenseAccount:  "expense_account",
		StepIncomeAmount:    "income_amount",
		StepIncomeAccount:   "income_account",
		StepMoveSource:      "move_source",
		StepMoveTarget:      "move_target",
		StepMoveAmount:      "move_amount",
		StepBudgetAmount:    "budget_amount",
		StepBudgetInterval:  "budget_interval",
		StepCreateEmoji:     "create_emoji",
		StepCreateName:      "create_name",
		StepCreateGroup:     "create_group",
		StepCreateCurrency:  "create_currency",
	}
	if n, ok := names[s]; ok {
		return n
	}
	return "unknown"
}

// ─── Runtime state ─────────────────────────────────────────

// userState holds the wizard and shared context for a single user.
type userState struct {
	Wizard Wizard     // discriminated union — only one variant active
	Step   WizardStep // current step within the wizard
	MsgID  int        // template message ID for in-place editing
	ChatID int64      // chat where the template lives
	Prev   string     // previous step label for back navigation
}

func (s *userState) IsWizard() bool {
	return s != nil && s.Step != StepIdle
}

func (s *userState) IsTextStep() bool {
	if s == nil {
		return false
	}
	switch s.Step {
	case StepExpenseAmount, StepIncomeAmount, StepMoveAmount,
		StepBudgetAmount, StepCreateName:
		return true
	}
	return false
}

// expenseW returns the wizard as ExpenseWizard value. Mutate and reassign to state.Wizard.
func (s *userState) expenseW() ExpenseWizard { return s.Wizard.(ExpenseWizard) }
func (s *userState) incomeW() IncomeWizard   { return s.Wizard.(IncomeWizard) }
func (s *userState) moveW() MoveWizard       { return s.Wizard.(MoveWizard) }
func (s *userState) budgetW() BudgetWizard   { return s.Wizard.(BudgetWizard) }
func (s *userState) creationW() CreationWizard { return s.Wizard.(CreationWizard) }
