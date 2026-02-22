PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS groups (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    telegram_chat_id INTEGER NOT NULL UNIQUE,
    default_currency TEXT    NOT NULL DEFAULT 'USD',
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS members (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id         INTEGER NOT NULL REFERENCES groups(id),
    telegram_user_id INTEGER NOT NULL,
    username         TEXT    NOT NULL DEFAULT '',
    first_name       TEXT    NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(group_id, telegram_user_id)
);

CREATE INDEX idx_members_group ON members(group_id);
CREATE INDEX idx_members_telegram_user ON members(telegram_user_id);

CREATE TABLE IF NOT EXISTS transactions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id            INTEGER NOT NULL REFERENCES groups(id),
    payer_member_id     INTEGER NOT NULL REFERENCES members(id),
    type                TEXT    NOT NULL CHECK(type IN ('personal_expense','split_equal','split_except','split_partial','lend','repay')),
    amount_cents        INTEGER NOT NULL,
    original_amount     INTEGER NOT NULL,
    original_currency   TEXT    NOT NULL,
    exchange_rate       REAL    NOT NULL,
    description         TEXT    NOT NULL DEFAULT '',
    transaction_date    DATE    NOT NULL DEFAULT (date('now')),
    telegram_message_id INTEGER,
    cancelled_at        DATETIME,
    created_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_transactions_group ON transactions(group_id);
CREATE INDEX idx_transactions_payer ON transactions(payer_member_id);
CREATE INDEX idx_transactions_date ON transactions(transaction_date);

CREATE TABLE IF NOT EXISTS transaction_participants (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id  INTEGER NOT NULL REFERENCES transactions(id),
    member_id       INTEGER NOT NULL REFERENCES members(id),
    amount_cents    INTEGER NOT NULL,
    UNIQUE(transaction_id, member_id)
);

CREATE INDEX idx_participants_transaction ON transaction_participants(transaction_id);
CREATE INDEX idx_participants_member ON transaction_participants(member_id);

CREATE TABLE IF NOT EXISTS transaction_line_items (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id  INTEGER NOT NULL REFERENCES transactions(id),
    member_id       INTEGER NOT NULL REFERENCES members(id),
    amount_cents    INTEGER NOT NULL,
    description     TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX idx_line_items_transaction ON transaction_line_items(transaction_id);
