package models

import (
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
	Name      string    `json:"name"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
	// Computed
	Spent     float64 `json:"spent,omitempty"`
	Budget    float64 `json:"budget,omitempty"`
	Remaining float64 `json:"remaining,omitempty"`
}

type Transaction struct {
	ID                int64      `json:"id"`
	UserID            int64      `json:"user_id"`
	AccountID         int64      `json:"account_id"`
	CategoryID        *int64     `json:"category_id,omitempty"`
	Type              string     `json:"type"` // expense, income, transfer
	Amount            float64    `json:"amount"`
	TransferAccountID *int64     `json:"transfer_account_id,omitempty"`
	Description       string     `json:"description,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	// Joined
	CategoryName      string     `json:"category_name,omitempty"`
	CategoryEmoji     string     `json:"category_emoji,omitempty"`
	AccountName       string     `json:"account_name,omitempty"`
}

type Budget struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	CategoryID int64     `json:"category_id"`
	Month      string    `json:"month"` // "2026-07"
	Amount     float64   `json:"amount"`
	CreatedAt  time.Time `json:"created_at"`
}
