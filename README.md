# Kaptl — YNAB-style Telegram Expense Tracker Bot

A Telegram bot for personal finance tracking with YNAB-style envelope budgeting.

## Features
- 💰 Multi-account support (checking, savings, cash, credit cards)
- 🏷️ Flat categories with emoji, user-created
- 🎯 Monthly per-category budgets with remaining tracking
- 👆 Inline keyboard wizard: pick category → type amount → pick account
- 🔒 Single-user auth (locked to Telegram ID)
- 🐘 PostgreSQL backend
- 🐳 Docker image, K8s-ready

## Quick Start

1. Set up a PostgreSQL database and run `migrations/001_init.sql`
2. Create `.env` with:
   ```
   TELEGRAM_BOT_TOKEN=your_bot_token
   ALLOWED_TELEGRAM_ID=your_telegram_id
   DATABASE_URL=postgres://user:pass@host:5432/ynab?sslmode=disable
   ```
3. `go run ./cmd/bot`

## Bot Commands

| Command | Description |
|---------|-------------|
| `/start` | Main menu |
| `➕ Expense` | Log an expense (wizard) |
| `💵 Income` | Log income |
| `📊 Summary` | Monthly budget overview |
| `🎯 Budgets` | Set monthly budget per category |
| `💰 Accounts` | View accounts with balances |
| `🏷️ Categories` | Manage categories |
| `📋 Recent` | Last 10 transactions |
| `+cat 🍞 Name` | Create category |
| `-cat Name` | Delete category |
| `+account Name type` | Create account |
