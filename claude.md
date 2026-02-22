# claude.md — Trip Ledger

> This file is the source of truth for the agent. **Update this file before every commit.** Log all changes in the [Changelog](#changelog) section.

---

## Project Overview

**Trip Ledger** is a Telegram group bot that tracks shared travel expenses among a group of friends. It records who paid what, splits bills under various scenarios, tracks debts between members, and produces spending summaries.

---

## Stack

| Layer | Technology |
|---|---|
| Language | Go |
| Database | SQLite (WAL mode, foreign keys ON) |
| Telegram API | `go-telegram-bot-api` |
| Project layout | [golang-standards/project-layout](https://github.com/golang-standards/project-layout) |

---

## Project Layout

```
trip-ledger/
├── cmd/
│   └── bot/              # main entrypoint
├── internal/
│   ├── bot/              # Telegram bot handlers (interface layer)
│   ├── service/          # business logic (abstract, interface-driven)
│   ├── store/            # SQLite repository implementations
│   └── model/            # shared domain types / DTOs
├── pkg/
│   └── currency/         # exchange rate fetching + in-memory cache
├── migrations/           # SQL migration files
├── docker-compose.yml
├── Dockerfile
├── .github/
│   └── workflows/
│       ├── pr.yml        # lint + test + build on PR
│       └── deploy.yml    # build image, push GHCR, SSH deploy on push to main
└── claude.md
```

---

## Architecture

- **Service layer** is fully decoupled from the Telegram interface. All business logic lives in `internal/service` and operates on plain Go types.
- All service dependencies (stores, currency client) are injected via **interfaces** to enable mocking in tests.
- The bot layer in `internal/bot` translates Telegram updates into service calls and formats responses.
- New commands or interfaces (e.g. REST API) can be added without touching the service layer.

---

## Data Model

Six tables in SQLite:

| Table | Purpose |
|---|---|
| `groups` | One row per Telegram group chat; holds `default_currency` |
| `members` | Telegram user within a group; identified by `telegram_user_id` + `group_id` |
| `transactions` | Central ledger entry; all amounts stored in **USD cents** (int64) |
| `transaction_participants` | One row per member who owes a share from a transaction |
| `transaction_line_items` | Personal line items pulled out before splitting (`split_except` only) |

### Key storage rules
- All monetary values stored as **integers (cents)** — no floats in the DB.
- Every transaction is converted to USD at creation time using the live exchange rate.
- `original_amount`, `original_currency`, and `exchange_rate` are stored to reconstruct display values in any currency.
- `cancelled_at` is a soft-delete field; cancelled transactions are excluded from all calculations.
- `telegram_message_id` on a transaction links the bot's confirmation reply to enable `/cancel`.

### Transaction types

| Type | Description |
|---|---|
| `personal_expense` | Payer spent on themselves only |
| `split_equal` | Payer covered the whole group; everyone else owes an equal share |
| `split_except` | Split equally after subtracting named personal line items |
| `split_partial` | Split equally among a named subset of members |
| `lend` | Payer lent money to one other member |
| `repay` | Borrower repaid the lender (partially or in full) |

---

## Currency Handling

- Exchange rates are fetched from an external API and cached **in memory**.
- Cache is **reset every hour**.
- Default currency per group is set via `/setcurrency`; falls back to `USD`.
- All reports are output in the group's default currency, converted at output time using the current rate.
- Explicit currency codes (ISO 4217) override the default in any command.

---

## Bot Commands

### Argument formats
- `[@user]` — optional Telegram username, e.g. `@john` (must exist in the group)
- `amount` — decimal number, e.g. `45.50`
- `[currency]` — ISO 4217 code, e.g. `EUR`
- `[date]` — `dd-mm-yy` or `dd-mm`; omit to use today

### Command reference

```
/expense [@user] amount [currency] [date] description
    Registers a personal expense for a user.
    If @user omitted: invoking user is the payer.

/split [@user] amount [currency] [date] description
    Splits total equally among ALL group members.
    If @user omitted: invoking user paid.

/splitexcept @user1:item_cost [@user2:item_cost ...] amount [currency] description
    Splits (total − personal items) equally among all members.
    Personal items are charged only to the tagged user.

/partial @user1 @user2 [...] amount [currency] description
    Splits equally among the mentioned users only.

/lend @from @to amount [currency] [description]
    Records that @from lent money to @to.

/repay @from @to [amount] [currency]
    Records repayment from @from to @to.
    If amount omitted: repaid in full.

/debts [@user]
    Shows current debt summary.
    If @user omitted: shows invoking user's debts.

/spending [@user] [from_date] [to_date] [currency]
    Shows spending breakdown for a period.
    If @user omitted: shows invoking user's spending.

/setcurrency currency_code
    Sets the default currency for all subsequent commands in this group.
    Default is USD.

/cancel   (reply to a successful transaction confirmation message)
    Soft-deletes the transaction. Cannot be undone.
```

---

## Error Handling

- Provide user-facing error messages for every top-level edge case (unknown user, bad amount, unknown currency, missing arguments, etc.).
- Log unexpected/internal errors server-side; do not expose stack traces to users.

---

## Testing

| Scope | Requirement |
|---|---|
| Unit tests | All service layer methods, using mock stores |
| Table-driven tests | Debt calculation logic |
| Integration test | At minimum one test using an in-memory SQLite instance |

---

## CI/CD

### On Pull Request
- `golangci-lint` (with `errcheck`, `govet`, `staticcheck`)
- `go test ./...`
- `go build`

### On push to `main`
1. Build Docker image
2. Push to GHCR (GitHub Container Registry)
3. SSH into VPS and run:
   ```
   docker-compose pull && docker-compose up -d
   ```

---

## Changelog

| Date | Author | Change |
|---|---|---|
| initial | agent | Created claude.md from project specification |

## Agent Workflow

### Feature completion marker

When you finish implementing a feature (all files written, the feature is functionally complete, and you are about to stop), you **must** output the exact string `FEATURE_COMPLETE` on its own line at the end of your response.

This marker triggers the automated ship cycle hook, which will run `/ship` (simplify → test → deploy → update docs) automatically.

Do **not** output `FEATURE_COMPLETE` if:
- You stopped mid-implementation to ask a question
- The build is currently broken
- You are only making a partial change and more work is expected

Example of correct usage:
```
...finished implementing /split command and wired it to the service layer.

FEATURE_COMPLETE
```