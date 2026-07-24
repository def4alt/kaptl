# Kaptl — YNAB-style Telegram Expense Tracker Bot

A Telegram bot for personal finance tracking with YNAB-style envelope budgeting.

## Features
- 💰 Multi-account (checking, savings, cash, credit cards)
- 🏷️ Categories with emoji, user-created
- 🎯 Monthly per-category budgets with remaining tracking
- 👆 Inline keyboard wizard for quick expense/income logging
- 🔒 Single-user auth (locked to Telegram ID)
- 🐘 PostgreSQL backend (CNPG on k3s)

## Slash Commands

| Command | Description |
|---------|-------------|
| `/start` | Show main menu |
| `/help` | Show all commands |
| `/cat add 🍞 Name` | Create a category |
| `/cat rm Name` | Delete a category |
| `/cat list` | List all categories |
| `/acc add Name type` | Create an account (types: checking, savings, cash, credit_card) |
| `/acc list` | List all accounts |
| `/budget set Name 5000` | Set monthly budget for a category |

## Button Actions

| Button | Description |
|--------|-------------|
| ➕ Expense | 3-tap wizard: category → amount → account |
| 💵 Income | 2-tap wizard: amount → account |
| 📊 Summary | Current month budget vs spending |
| 🎯 Budgets | View/set monthly budgets |
| 💰 Accounts | View accounts with balances |
| 🏷️ Categories | View categories |
| 📋 Recent | Last 10 transactions |

## Quick Start

1. `cp .env.example .env` and fill in your values
2. `go run ./cmd/bot`
