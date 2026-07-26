# Currency and reporting model

Kaptl separates native ledger facts from reporting valuations. A transaction such as `721.00 UAH` is never replaced by its EUR estimate.

## Native ledger invariants

- Supported currencies are `EUR`, `USD`, `UAH`, `PLN`, and `GBP`.
- Go uses decimal arithmetic and PostgreSQL uses `NUMERIC`; binary floating point is not used for money or FX rates.
- An account owns an immutable currency.
- PostgreSQL derives a transaction's currency from its source account.
- Transaction valuation inputs—user, source account, type, amount, currency, transfer destination, and timestamp—are immutable.
- Account, category, transfer, and budget references remain tenant-scoped.
- Native account balances are calculated and displayed independently by currency.
- Cross-currency transfers remain rejected. A future FX transfer requires separate source and destination amounts, fees, and settlement provenance.

## Reporting currency and budgets

`users.reporting_currency` is an immutable per-user setting. Policy version 1 fixes it to `EUR`; later currencies require a new provider/routing policy and migration.

Budgets are reporting-currency envelopes. The bot does not accept a budget currency from the caller; PostgreSQL snapshots the user's reporting currency. Consequently, a UAH expense can reduce an EUR budget only after its explicit historical EUR valuation exists.

Migration `003` deliberately aborts if a legacy budget is not already in the user's reporting currency. Operators must audit and resolve such budgets explicitly rather than relabeling them or inventing a conversion rate; the production preflight verifies this invariant before migration.

Summary rows, rollover, and Ready to Assign are calculated while holding the per-user advisory lock. Read-committed statements see any writer that completed before lock acquisition, while the lock prevents financial facts from changing during the report. A report never silently omits unvalued transactions: if any relevant expense or income lacks a valuation, the whole reporting summary returns a pending status and rollover is not advanced.

## Historical valuations

`transaction_valuations` stores immutable, versioned reporting snapshots separate from `transactions`:

- transaction ID;
- target currency;
- purpose (`budget` in policy version 1);
- policy version;
- rounded reporting amount;
- exact FX quote reference.

`fx_quotes` records source and target currencies, an explicitly directed decimal rate, provider, effective date, and observation timestamp. Conversion is always:

```text
target amount = source amount × rate
```

The final target amount is rounded once using the target currency's minor-unit exponent and banker's rounding. Native amounts and high-precision rates are not rounded during intermediate arithmetic.

EUR-to-EUR valuations use an explicit identity quote. Foreign-currency valuations currently use official National Bank of Ukraine rates. NBU is used because the ECB daily reference-rate dataset does not publish a bilateral UAH/EUR series. USD, GBP, and PLN are triangulated through their official UAH rates and the official EUR/UAH rate for the same effective date.

The rate policy accepts only the latest official effective date at or before the transaction's Europe/Kyiv calendar date, with a maximum seven-day fallback window. No parity, zero, future, or indefinitely stale rate is invented. Existing cached quotes are insert-only; a provider revision conflicting with an already cached quote is rejected rather than rewriting history.

These valuations are personal historical reporting estimates. They do not claim to be the bank's settlement rate, spread, fee, tax rate, or current market value.

## Acquisition and failure behavior

Native transaction insertion never performs an HTTP request. An `AFTER INSERT` trigger atomically creates a durable valuation job for each expense or income. The background worker:

1. leases a job with `FOR UPDATE SKIP LOCKED`;
2. reuses an immutable quote only for the exact requested calendar date, otherwise asks the provider to resolve the applicable bounded historical fallback;
3. converts with decimal arithmetic;
4. acquires the same per-user advisory lock used by reporting rollover;
5. atomically stores the quote and valuation and retains the durable job in `completed` state.

Provider or network failure leaves the native transaction intact and retries with bounded exponential backoff. Worker leases expire after crashes. Telegram views remain pure: they format native and already-computed reporting amounts but never fetch rates.

## Historical versus current value

Policy version 1 implements transaction-date historical valuation for spending, budgets, and Ready to Assign. It does not implement current mark-to-market account valuation. A future current-value report must be separately named, timestamped, and calculated using current quotes; it must not mutate historical transaction valuations.
