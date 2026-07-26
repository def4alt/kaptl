package models

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type User struct {
	TelegramID        int64     `json:"telegram_id"`
	Username          string    `json:"username"`
	FirstName         string    `json:"first_name"`
	LanguageCode      string    `json:"language_code"`
	ReportingCurrency string    `json:"reporting_currency"`
	CreatedAt         time.Time `json:"created_at"`
}

type Account struct {
	ID             int64           `json:"id"`
	UserID         int64           `json:"user_id"`
	Name           string          `json:"name"`
	Emoji          string          `json:"emoji"`
	Currency       string          `json:"currency"`
	InitialBalance decimal.Decimal `json:"initial_balance"`
	CreatedAt      time.Time       `json:"created_at"`
	// Computed
	Balance decimal.Decimal `json:"balance,omitempty"`
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
	ID                int64           `json:"id"`
	UserID            int64           `json:"user_id"`
	Name              string          `json:"name"`
	Emoji             string          `json:"emoji"`
	Currency          string          `json:"currency"`
	CreatedAt         time.Time       `json:"created_at"`
	Spent             decimal.Decimal `json:"spent"`
	Budget            decimal.Decimal `json:"budget"`
	Rollover          decimal.Decimal `json:"rollover"`
	Available         decimal.Decimal `json:"available"`
	Remaining         decimal.Decimal `json:"remaining"`
	MissingValuations int             `json:"missing_valuations"`
	GroupName         string          `json:"group_name,omitempty"`
	GroupID           *int64          `json:"group_id,omitempty"`
}

type CurrencyAmount struct {
	Currency          string          `json:"currency"`
	Amount            decimal.Decimal `json:"amount"`
	MissingValuations int             `json:"missing_valuations"`
}

type ReportingSummary struct {
	Currency      string
	Rows          []BudgetRow
	ReadyToAssign []CurrencyAmount
}

type ValuationsPendingError struct {
	Count  int
	Failed int
}

func (e *ValuationsPendingError) Error() string {
	if e.Failed > 0 {
		return fmt.Sprintf("%d transaction valuation(s) unavailable; %d exhausted retries", e.Count, e.Failed)
	}
	return fmt.Sprintf("%d transaction valuation(s) pending", e.Count)
}

type Transaction struct {
	ID                int64           `json:"id"`
	UserID            int64           `json:"user_id"`
	AccountID         int64           `json:"account_id"`
	CategoryID        *int64          `json:"category_id,omitempty"`
	Type              string          `json:"type"` // expense, income, transfer
	Amount            decimal.Decimal `json:"amount"`
	Currency          string          `json:"currency"`
	TransferAccountID *int64          `json:"transfer_account_id,omitempty"`
	Description       string          `json:"description,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	// Joined
	CategoryName      string           `json:"category_name,omitempty"`
	CategoryEmoji     string           `json:"category_emoji,omitempty"`
	AccountName       string           `json:"account_name,omitempty"`
	ReportingAmount   *decimal.Decimal `json:"reporting_amount,omitempty"`
	ReportingCurrency string           `json:"reporting_currency,omitempty"`
}

type Budget struct {
	ID             int64           `json:"id"`
	UserID         int64           `json:"user_id"`
	CategoryID     int64           `json:"category_id"`
	Currency       string          `json:"currency"`
	PeriodStart    time.Time       `json:"period_start"`
	IntervalDays   int             `json:"interval_days"`
	IntervalMonths int             `json:"interval_months"`
	Amount         decimal.Decimal `json:"amount"`
	Rollover       decimal.Decimal `json:"rollover"`
	CreatedAt      time.Time       `json:"created_at"`
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
