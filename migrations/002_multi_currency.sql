BEGIN;

UPDATE accounts SET currency = UPPER(currency);

ALTER TABLE transactions ADD COLUMN IF NOT EXISTS currency TEXT;
UPDATE transactions t
SET currency = UPPER(a.currency)
FROM accounts a
WHERE a.id = t.account_id
  AND t.currency IS NULL;
ALTER TABLE transactions ALTER COLUMN currency SET NOT NULL;

ALTER TABLE budgets ADD COLUMN IF NOT EXISTS currency TEXT;
UPDATE budgets SET currency = 'EUR' WHERE currency IS NULL;
UPDATE budgets SET currency = UPPER(currency);
ALTER TABLE budgets ALTER COLUMN currency SET DEFAULT 'EUR';
ALTER TABLE budgets ALTER COLUMN currency SET NOT NULL;

ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_user_id_category_id_key;
ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_positive_interval_check;

DO $audit$
BEGIN
    IF EXISTS (SELECT 1 FROM accounts WHERE currency NOT IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')) THEN
        RAISE EXCEPTION 'unsupported account currency found';
    END IF;
    IF EXISTS (
        SELECT 1 FROM transactions t
        LEFT JOIN accounts a ON a.id = t.account_id AND a.user_id = t.user_id
        WHERE a.id IS NULL
    ) THEN
        RAISE EXCEPTION 'transaction source-account ownership mismatch found';
    END IF;
    IF EXISTS (
        SELECT 1 FROM transactions t
        LEFT JOIN categories c ON c.id = t.category_id AND c.user_id = t.user_id
        WHERE t.category_id IS NOT NULL AND c.id IS NULL
    ) THEN
        RAISE EXCEPTION 'transaction category ownership mismatch found';
    END IF;
    IF EXISTS (
        SELECT 1 FROM transactions t
        LEFT JOIN accounts a ON a.id = t.transfer_account_id AND a.user_id = t.user_id
        WHERE t.type = 'transfer'
          AND (t.category_id IS NOT NULL OR t.transfer_account_id = t.account_id
               OR (t.transfer_account_id IS NOT NULL AND (a.id IS NULL OR a.currency <> t.currency)))
    ) THEN
        RAISE EXCEPTION 'invalid or cross-currency transfer found';
    END IF;
    IF EXISTS (
        SELECT 1 FROM transactions
        WHERE type <> 'transfer' AND transfer_account_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'invalid transaction type/category/destination shape found';
    END IF;
    IF EXISTS (
        SELECT 1 FROM budgets b
        LEFT JOIN categories c ON c.id = b.category_id AND c.user_id = b.user_id
        WHERE c.id IS NULL
    ) THEN
        RAISE EXCEPTION 'budget category ownership mismatch found';
    END IF;
    IF EXISTS (
        SELECT 1 FROM categories c
        LEFT JOIN category_groups g ON g.id = c.group_id AND g.user_id = c.user_id
        WHERE c.group_id IS NOT NULL AND g.id IS NULL
    ) THEN
        RAISE EXCEPTION 'category group ownership mismatch found';
    END IF;
    IF EXISTS (
        SELECT 1 FROM budgets
        WHERE interval_days < 0 OR interval_days > 3650
           OR interval_months < 0 OR interval_months > 120
           OR (interval_days = 0 AND interval_months = 0)
    ) THEN
        RAISE EXCEPTION 'invalid budget interval found';
    END IF;
END
$audit$;

DO $migration$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'accounts_currency_code_check' AND conrelid = 'accounts'::regclass
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_currency_code_check
            CHECK (currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'transactions_currency_code_check' AND conrelid = 'transactions'::regclass
    ) THEN
        ALTER TABLE transactions
            ADD CONSTRAINT transactions_currency_code_check
            CHECK (currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'budgets_currency_code_check' AND conrelid = 'budgets'::regclass
    ) THEN
        ALTER TABLE budgets
            ADD CONSTRAINT budgets_currency_code_check
            CHECK (currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'budgets_positive_interval_check' AND conrelid = 'budgets'::regclass
    ) THEN
        ALTER TABLE budgets
            ADD CONSTRAINT budgets_positive_interval_check
            CHECK (
                interval_days BETWEEN 0 AND 3650
                AND interval_months BETWEEN 0 AND 120
                AND (interval_days > 0 OR interval_months > 0)
            );
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'budgets_user_category_currency_key' AND conrelid = 'budgets'::regclass
    ) THEN
        ALTER TABLE budgets
            ADD CONSTRAINT budgets_user_category_currency_key
            UNIQUE (user_id, category_id, currency);
    END IF;
END
$migration$;

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

CREATE INDEX IF NOT EXISTS idx_transactions_category_currency_date
    ON transactions(category_id, currency, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_budgets_user_currency
    ON budgets(user_id, currency);

COMMIT;
