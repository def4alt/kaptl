# Currency model

Kaptl does not convert money implicitly. Every arithmetic operation is scoped to one currency.

## Invariants

- Supported currencies are `EUR`, `USD`, `UAH`, `PLN`, and `GBP`. They all use two decimal places in the current amount model.
- An account owns its currency. Its currency is immutable because the application has no currency-update operation or conversion model.
- A transaction stores an immutable currency snapshot derived by PostgreSQL from its source account. Callers do not choose it.
- Transaction account, category, and transfer references must belong to the transaction user. Database triggers enforce this at the write boundary.
- A budget is identified by `(user_id, category_id, currency)`. A category can therefore have independent budgets in several currencies.
- Summary, account, spending, budget, and Ready-to-Assign totals are aggregated and rendered per currency. There is no cross-currency grand total.
- Unbudgeted spending is included as a separate `(category, currency)` summary row for the current calendar month.
- Transfers are allowed only between accounts with the same currency. Cross-currency movement requires a future conversion model with source amount, destination amount, rate, and rate timestamp.

## Persistence

`transactions.currency` is a historical snapshot rather than a join-time derivation. This prevents an account metadata change from rewriting transaction history. The database trigger also rejects attempts to mutate a transaction's user, source account, or currency.

Existing transactions are backfilled from their source accounts by `002_multi_currency.sql`. Existing budgets are explicitly migrated to `EUR`, preserving their previous semantics.

## Rollover

Budget rollover is serialized against transaction inserts with a per-user PostgreSQL advisory lock and processed one closed interval at a time. PostgreSQL computes the transaction timestamp and interval boundaries. Spending is bounded by `[period_start, period_end)` and filtered by the budget currency. Summary generation and rollover commit in one transaction.

## Future conversion support

Do not add exchange rates to summary formatting or convert using a current rate. A conversion feature needs a domain object that records both monetary legs and the rate used at transaction time. Only then can a separate reporting policy convert historical values into a chosen base currency.
