-- Historical reporting valuations without mutating native ledger amounts.
BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS reporting_currency TEXT NOT NULL DEFAULT 'EUR';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_reporting_currency_code_check'
          AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users ADD CONSTRAINT users_reporting_currency_code_check
            CHECK (reporting_currency = 'EUR');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS fx_quotes (
    id BIGSERIAL PRIMARY KEY,
    source_currency TEXT NOT NULL CHECK (source_currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')),
    target_currency TEXT NOT NULL CHECK (target_currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')),
    rate NUMERIC(38,20) NOT NULL CHECK (rate > 0),
    effective_at DATE NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    provider TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT fx_quotes_distinct_or_identity_check CHECK (
        source_currency <> target_currency OR rate = 1
    ),
    UNIQUE(provider, source_currency, target_currency, effective_at)
);

CREATE TABLE IF NOT EXISTS transaction_valuations (
    transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    target_currency TEXT NOT NULL CHECK (target_currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')),
    purpose TEXT NOT NULL CHECK (purpose IN ('budget')),
    policy_version INTEGER NOT NULL CHECK (policy_version > 0),
    amount NUMERIC(18,2) NOT NULL CHECK (amount >= 0),
    fx_quote_id BIGINT NOT NULL REFERENCES fx_quotes(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT transaction_valuations_policy_shape_check CHECK (
        policy_version <> 1 OR (purpose = 'budget' AND target_currency = 'EUR')
    ),
    PRIMARY KEY(transaction_id, target_currency, purpose, policy_version)
);

CREATE TABLE IF NOT EXISTS valuation_jobs (
    transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    target_currency TEXT NOT NULL CHECK (target_currency IN ('EUR', 'USD', 'UAH', 'PLN', 'GBP')),
    purpose TEXT NOT NULL CHECK (purpose IN ('budget')),
    policy_version INTEGER NOT NULL CHECK (policy_version > 0),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'retry', 'failed', 'completed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    locked_until TIMESTAMPTZ,
    locked_by TEXT,
    locked_token TEXT,
    last_error TEXT,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT valuation_jobs_policy_shape_check CHECK (
        policy_version <> 1 OR (purpose = 'budget' AND target_currency = 'EUR')
    ),
    CONSTRAINT valuation_jobs_lock_fields_check CHECK (
        (locked_by IS NULL AND locked_until IS NULL AND locked_token IS NULL)
        OR (locked_by IS NOT NULL AND locked_until IS NOT NULL AND locked_token IS NOT NULL)
    ),
    CONSTRAINT valuation_jobs_terminal_state_check CHECK (
        (status IN ('pending', 'retry') AND completed_at IS NULL AND failed_at IS NULL)
        OR (status = 'completed' AND completed_at IS NOT NULL AND failed_at IS NULL
            AND locked_by IS NULL AND locked_until IS NULL AND locked_token IS NULL)
        OR (status = 'failed' AND failed_at IS NOT NULL AND completed_at IS NULL
            AND locked_by IS NULL AND locked_until IS NULL AND locked_token IS NULL)
    ),
    PRIMARY KEY(transaction_id, target_currency, purpose, policy_version)
);

CREATE INDEX IF NOT EXISTS idx_valuation_jobs_due
    ON valuation_jobs(status, next_attempt_at, transaction_id);
CREATE INDEX IF NOT EXISTS idx_transaction_valuations_target
    ON transaction_valuations(target_currency, purpose, transaction_id);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM budgets b
        JOIN users u ON u.telegram_id = b.user_id
        WHERE b.currency <> u.reporting_currency
    ) THEN
        RAISE EXCEPTION 'existing budget currency differs from user reporting currency';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION enforce_user_reporting_currency_immutable() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.reporting_currency IS DISTINCT FROM OLD.reporting_currency THEN
        RAISE EXCEPTION 'reporting currency is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS users_reporting_currency_immutable ON users;
CREATE TRIGGER users_reporting_currency_immutable
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION enforce_user_reporting_currency_immutable();

CREATE OR REPLACE FUNCTION enforce_budget_reporting_currency() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.currency IS DISTINCT FROM (
        SELECT reporting_currency FROM users WHERE telegram_id = NEW.user_id
    ) THEN
        RAISE EXCEPTION 'budget currency must equal user reporting currency';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS budgets_reporting_currency_guard ON budgets;
CREATE TRIGGER budgets_reporting_currency_guard
BEFORE INSERT OR UPDATE OF currency, user_id ON budgets
FOR EACH ROW EXECUTE FUNCTION enforce_budget_reporting_currency();

CREATE OR REPLACE FUNCTION enforce_transaction_valuation_inputs_immutable() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.user_id IS DISTINCT FROM OLD.user_id
       OR NEW.account_id IS DISTINCT FROM OLD.account_id
       OR NEW.type IS DISTINCT FROM OLD.type
       OR NEW.amount IS DISTINCT FROM OLD.amount
       OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.category_id IS DISTINCT FROM OLD.category_id
       OR NEW.transfer_account_id IS DISTINCT FROM OLD.transfer_account_id
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'transaction valuation inputs are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS transactions_valuation_inputs_immutable ON transactions;
CREATE TRIGGER transactions_valuation_inputs_immutable
BEFORE UPDATE ON transactions
FOR EACH ROW EXECUTE FUNCTION enforce_transaction_valuation_inputs_immutable();

CREATE OR REPLACE FUNCTION enforce_fx_quote_immutable() RETURNS TRIGGER AS $$
BEGIN
    IF NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'FX quotes are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS fx_quotes_immutable ON fx_quotes;
CREATE TRIGGER fx_quotes_immutable
BEFORE UPDATE ON fx_quotes
FOR EACH ROW EXECUTE FUNCTION enforce_fx_quote_immutable();

CREATE OR REPLACE FUNCTION round_half_even_2(value NUMERIC) RETURNS NUMERIC AS $$
DECLARE
    absolute_scaled NUMERIC := abs(value) * 100;
    whole NUMERIC := trunc(absolute_scaled);
    fraction NUMERIC := absolute_scaled - whole;
    rounded_scaled NUMERIC;
BEGIN
    IF fraction < 0.5 THEN
        rounded_scaled := whole;
    ELSIF fraction > 0.5 THEN
        rounded_scaled := whole + 1;
    ELSIF mod(whole, 2) = 0 THEN
        rounded_scaled := whole;
    ELSE
        rounded_scaled := whole + 1;
    END IF;
    RETURN sign(value) * rounded_scaled / 100;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION enforce_transaction_valuation_linkage() RETURNS TRIGGER AS $$
DECLARE
    native_currency TEXT;
    native_amount NUMERIC;
    transaction_date DATE;
    quote_source TEXT;
    quote_target TEXT;
    quote_date DATE;
    quote_rate NUMERIC;
BEGIN
    IF TG_OP = 'UPDATE' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'transaction valuations are immutable';
    END IF;

    SELECT t.currency, t.amount, (t.created_at AT TIME ZONE 'Europe/Kyiv')::date,
           q.source_currency, q.target_currency, q.effective_at, q.rate
      INTO native_currency, native_amount, transaction_date, quote_source, quote_target, quote_date, quote_rate
    FROM transactions t
    JOIN fx_quotes q ON q.id = NEW.fx_quote_id
    WHERE t.id = NEW.transaction_id;

    IF quote_source IS NULL
       OR quote_source <> native_currency
       OR quote_target <> NEW.target_currency THEN
        RAISE EXCEPTION 'quote currencies do not match transaction valuation';
    END IF;
    IF quote_date > transaction_date OR quote_date < transaction_date - 7 THEN
        RAISE EXCEPTION 'quote effective date is outside valuation policy window';
    END IF;
    IF NEW.amount <> round_half_even_2(native_amount * quote_rate) THEN
        RAISE EXCEPTION 'valuation amount does not match native amount and quote rate';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS transaction_valuations_linkage_guard ON transaction_valuations;
CREATE TRIGGER transaction_valuations_linkage_guard
BEFORE INSERT OR UPDATE ON transaction_valuations
FOR EACH ROW EXECUTE FUNCTION enforce_transaction_valuation_linkage();

CREATE OR REPLACE FUNCTION enqueue_transaction_valuation() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO valuation_jobs (transaction_id, target_currency, purpose, policy_version)
    SELECT NEW.id, u.reporting_currency, 'budget', 1
    FROM users u
    WHERE u.telegram_id = NEW.user_id
      AND NEW.type IN ('expense', 'income')
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS transactions_enqueue_valuation ON transactions;
CREATE TRIGGER transactions_enqueue_valuation
AFTER INSERT ON transactions
FOR EACH ROW EXECUTE FUNCTION enqueue_transaction_valuation();

-- Idempotently queue all historical transactions. Native amount/currency columns
-- remain untouched; the worker creates separate, auditable valuations.
INSERT INTO valuation_jobs (transaction_id, target_currency, purpose, policy_version)
SELECT t.id, u.reporting_currency, 'budget', 1
FROM transactions t
JOIN users u ON u.telegram_id = t.user_id
LEFT JOIN transaction_valuations v
    ON v.transaction_id = t.id
   AND v.target_currency = u.reporting_currency
   AND v.purpose = 'budget'
   AND v.policy_version = 1
WHERE v.transaction_id IS NULL
  AND t.type IN ('expense', 'income')
ON CONFLICT DO NOTHING;

COMMIT;
