package models

import (
	"fmt"
	"time"
)

type User struct {
	TelegramID   int64     `json:"telegram_id"`
	Username     string    `json:"username"`
	FirstName    string    `json:"first_name"`
	LanguageCode string    `json:"language_code"`
	CreatedAt    time.Time `json:"created_at"`
}

type Account struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	Name           string    `json:"name"`
	Emoji          string    `json:"emoji"`
	Currency       string    `json:"currency"`
	InitialBalance float64   `json:"initial_balance"`
	CreatedAt      time.Time `json:"created_at"`
	// Computed
	Balance float64 `json:"balance,omitempty"`
}

type Category struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	GroupID   *int64    `json:"group_id,omitempty"`
	Name      string    `json:"name"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

// CategoryGroup groups categories together (e.g. "Needs & Musts").
type CategoryGroup struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Emoji     string    `json:"emoji"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// BudgetRow is a category with computed spending fields for the summary view.
type BudgetRow struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
	Spent     float64   `json:"spent"`
	Budget    float64   `json:"budget"`
	Rollover  float64   `json:"rollover"`
	Available float64   `json:"available"`
	Remaining float64   `json:"remaining"`
	GroupName string    `json:"group_name,omitempty"`
	GroupID   *int64    `json:"group_id,omitempty"`
}

type Transaction struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	AccountID         int64     `json:"account_id"`
	CategoryID        *int64    `json:"category_id,omitempty"`
	Type              string    `json:"type"` // expense, income, transfer
	Amount            float64   `json:"amount"`
	TransferAccountID *int64    `json:"transfer_account_id,omitempty"`
	Description       string    `json:"description,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	// Joined
	CategoryName  string `json:"category_name,omitempty"`
	CategoryEmoji string `json:"category_emoji,omitempty"`
	AccountName   string `json:"account_name,omitempty"`
}

type Budget struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	CategoryID     int64     `json:"category_id"`
	PeriodStart    time.Time `json:"period_start"`
	IntervalDays   int       `json:"interval_days"`
	IntervalMonths int       `json:"interval_months"`
	Amount         float64   `json:"amount"`
	Rollover       float64   `json:"rollover"`
	CreatedAt      time.Time `json:"created_at"`
}

// Description returns a human-readable interval label.
func (b Budget) Description() string {
	switch {
	case b.IntervalMonths >= 3:
		return fmt.Sprintf("every %d months", b.IntervalMonths)
	case b.IntervalMonths == 1:
		return "monthly"
	case b.IntervalDays >= 14:
		return fmt.Sprintf("every %d days", b.IntervalDays)
	case b.IntervalDays == 7:
		return "weekly"
	default:
		return fmt.Sprintf("every %d days", b.IntervalDays)
	}
}
