-- kaptl database schema
-- telegram_id is the primary key for users — no separate auto-increment id column.

CREATE TABLE IF NOT EXISTS users (
    telegram_id BIGINT PRIMARY KEY,
    username TEXT,
    first_name TEXT,
    language_code TEXT DEFAULT 'en',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS accounts (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(telegram_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    emoji TEXT NOT NULL DEFAULT '💳',
    currency TEXT NOT NULL DEFAULT 'EUR' CONSTRAINT accounts_currency_code_check CHECK (currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')),
    initial_balance DECIMAL(12,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS category_groups (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(telegram_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    emoji TEXT NOT NULL DEFAULT '📁',
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS categories (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(telegram_id) ON DELETE CASCADE,
    group_id INTEGER REFERENCES category_groups(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    emoji TEXT NOT NULL DEFAULT '📌',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS transactions (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(telegram_id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    type TEXT NOT NULL CHECK (type IN ('expense', 'income', 'transfer')),
    amount DECIMAL(12,2) NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL CONSTRAINT transactions_currency_code_check CHECK (currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')),
    transfer_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS budgets (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(telegram_id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    currency TEXT NOT NULL DEFAULT 'EUR' CONSTRAINT budgets_currency_code_check CHECK (currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')),
    period_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    interval_days INTEGER NOT NULL DEFAULT 0,
    interval_months INTEGER NOT NULL DEFAULT 1,
    rollover NUMERIC(12,2) NOT NULL DEFAULT 0,
    amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT budgets_positive_interval_check CHECK (
        interval_days BETWEEN 0 AND 3650
        AND interval_months BETWEEN 0 AND 120
        AND (interval_days > 0 OR interval_months > 0)
    ),
    CONSTRAINT budgets_user_category_currency_key UNIQUE(user_id, category_id, currency)
);

-- Currency is an immutable transaction snapshot derived from the source account.
-- Transfers without an exchange-rate model are valid only between equal currencies.
CREATE OR REPLACE FUNCTION enforce_transaction_currency() RETURNS TRIGGER AS $$
DECLARE
    source_currency TEXT;
    target_currency TEXT;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.user_id IS DISTINCT FROM OLD.user_id
            OR NEW.account_id IS DISTINCT FROM OLD.account_id
            OR NEW.currency IS DISTINCT FROM OLD.currency THEN
            RAISE EXCEPTION 'transaction user, source account, and currency are immutable';
        END IF;
        NEW.currency := OLD.currency;
    ELSE
        SELECT currency INTO source_currency
        FROM accounts
        WHERE id = NEW.account_id AND user_id = NEW.user_id;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'source account does not belong to transaction user';
        END IF;
        IF NEW.currency IS NOT NULL AND NEW.currency <> source_currency THEN
            RAISE EXCEPTION 'transaction currency must match source account';
        END IF;
        NEW.currency := source_currency;
    END IF;

    IF NEW.category_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM categories WHERE id = NEW.category_id AND user_id = NEW.user_id
    ) THEN
        RAISE EXCEPTION 'category does not belong to transaction user';
    END IF;

    IF NEW.type = 'transfer' THEN
        IF NEW.category_id IS NOT NULL OR NEW.transfer_account_id = NEW.account_id THEN
            RAISE EXCEPTION 'transfer requires distinct source/destination accounts and no category';
        END IF;
        IF NEW.transfer_account_id IS NULL THEN
            IF TG_OP = 'INSERT' OR OLD.transfer_account_id IS NULL THEN
                RAISE EXCEPTION 'transfer requires a destination account';
            END IF;
            RETURN NEW;
        END IF;
        SELECT currency INTO target_currency
        FROM accounts
        WHERE id = NEW.transfer_account_id AND user_id = NEW.user_id;
        IF NOT FOUND OR target_currency <> NEW.currency THEN
            RAISE EXCEPTION 'cross-currency transfers require explicit conversion data';
        END IF;
    ELSIF NEW.transfer_account_id IS NOT NULL THEN
        RAISE EXCEPTION 'only transfers may have a destination account';
    ELSIF TG_OP = 'INSERT' AND NEW.type = 'expense' AND NEW.category_id IS NULL THEN
        RAISE EXCEPTION 'expense requires a category';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS transactions_currency_guard ON transactions;
CREATE TRIGGER transactions_currency_guard
BEFORE INSERT OR UPDATE ON transactions
FOR EACH ROW EXECUTE FUNCTION enforce_transaction_currency();

CREATE OR REPLACE FUNCTION guard_account_currency() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.currency IS DISTINCT FROM OLD.currency THEN
        RAISE EXCEPTION 'account currency is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS accounts_currency_guard ON accounts;
CREATE TRIGGER accounts_currency_guard
BEFORE UPDATE OF currency ON accounts
FOR EACH ROW EXECUTE FUNCTION guard_account_currency();

CREATE OR REPLACE FUNCTION enforce_budget_ownership() RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM categories WHERE id = NEW.category_id AND user_id = NEW.user_id
    ) THEN
        RAISE EXCEPTION 'category does not belong to budget user';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS budgets_ownership_guard ON budgets;
CREATE TRIGGER budgets_ownership_guard
BEFORE INSERT OR UPDATE ON budgets
FOR EACH ROW EXECUTE FUNCTION enforce_budget_ownership();

CREATE OR REPLACE FUNCTION enforce_category_group_ownership() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.group_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM category_groups WHERE id = NEW.group_id AND user_id = NEW.user_id
    ) THEN
        RAISE EXCEPTION 'category group does not belong to category user';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS categories_group_ownership_guard ON categories;
CREATE TRIGGER categories_group_ownership_guard
BEFORE INSERT OR UPDATE OF user_id, group_id ON categories
FOR EACH ROW EXECUTE FUNCTION enforce_category_group_ownership();

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_transactions_user_date ON transactions(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transactions_account ON transactions(account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_category ON transactions(category_id);
CREATE INDEX IF NOT EXISTS idx_transactions_category_currency_date ON transactions(category_id, currency, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_budgets_user_currency ON budgets(user_id, currency);
