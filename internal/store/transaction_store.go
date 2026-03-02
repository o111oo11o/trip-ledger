package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/o111oo11o/trip-ledger/internal/model"
)

// scanRows iterates rows, calling scan for each row, and returns the collected results.
// It closes rows and propagates rows.Err() automatically.
func scanRows[T any](rows *sql.Rows, scan func(*sql.Rows) (T, error)) ([]T, error) {
	defer func() { _ = rows.Close() }()
	var result []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// SQLiteTransactionStore implements TransactionStore using SQLite.
type SQLiteTransactionStore struct {
	db *DB
}

func NewTransactionStore(db *DB) *SQLiteTransactionStore {
	return &SQLiteTransactionStore{db: db}
}

func (s *SQLiteTransactionStore) CreateTransaction(ctx context.Context, tx *model.Transaction) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO transactions
		 (group_id, payer_member_id, type, amount_cents, original_amount, original_currency,
		  exchange_rate, description, transaction_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tx.GroupID, tx.PayerMemberID, tx.Type, tx.AmountCents,
		tx.OriginalAmount, tx.OriginalCurrency, tx.ExchangeRate,
		tx.Description, tx.TransactionDate.Format("2006-01-02"),
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	tx.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteTransactionStore) AddParticipant(ctx context.Context, p *model.TransactionParticipant) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO transaction_participants (transaction_id, member_id, amount_cents)
		 VALUES (?, ?, ?)`,
		p.TransactionID, p.MemberID, p.AmountCents,
	)
	if err != nil {
		return fmt.Errorf("insert participant: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteTransactionStore) AddLineItem(ctx context.Context, li *model.TransactionLineItem) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO transaction_line_items (transaction_id, member_id, amount_cents, description)
		 VALUES (?, ?, ?, ?)`,
		li.TransactionID, li.MemberID, li.AmountCents, li.Description,
	)
	if err != nil {
		return fmt.Errorf("insert line item: %w", err)
	}
	li.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteTransactionStore) GetTransactionByMessageID(ctx context.Context, groupID, telegramMessageID int64) (*model.Transaction, error) {
	var tx model.Transaction
	err := s.db.QueryRowContext(ctx,
		`SELECT id, group_id, payer_member_id, type, amount_cents,
		        original_amount, original_currency, exchange_rate,
		        description, transaction_date, telegram_message_id, cancelled_at, created_at
		 FROM transactions
		 WHERE group_id = ? AND telegram_message_id = ? AND cancelled_at IS NULL`,
		groupID, telegramMessageID,
	).Scan(
		&tx.ID, &tx.GroupID, &tx.PayerMemberID, &tx.Type, &tx.AmountCents,
		&tx.OriginalAmount, &tx.OriginalCurrency, &tx.ExchangeRate,
		&tx.Description, &tx.TransactionDate, &tx.TelegramMessageID,
		&tx.CancelledAt, &tx.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("transaction not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query transaction by message id: %w", err)
	}
	return &tx, nil
}

func (s *SQLiteTransactionStore) CancelTransaction(ctx context.Context, transactionID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transactions SET cancelled_at = datetime('now') WHERE id = ?`,
		transactionID,
	)
	return err
}

func (s *SQLiteTransactionStore) SetTelegramMessageID(ctx context.Context, transactionID, messageID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transactions SET telegram_message_id = ? WHERE id = ?`,
		messageID, transactionID,
	)
	return err
}

func (s *SQLiteTransactionStore) GetActiveParticipants(ctx context.Context, groupID int64) ([]ParticipantWithPayer, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tp.member_id, t.payer_member_id, tp.amount_cents
		 FROM transaction_participants tp
		 JOIN transactions t ON t.id = tp.transaction_id
		 WHERE t.group_id = ? AND t.cancelled_at IS NULL AND t.type != 'repay'`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("query active participants: %w", err)
	}
	return scanRows(rows, func(r *sql.Rows) (ParticipantWithPayer, error) {
		var p ParticipantWithPayer
		if err := r.Scan(&p.ParticipantMemberID, &p.PayerMemberID, &p.AmountCents); err != nil {
			return p, fmt.Errorf("scan participant: %w", err)
		}
		return p, nil
	})
}

func (s *SQLiteTransactionStore) GetActiveRepayments(ctx context.Context, groupID int64) ([]RepaymentRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.payer_member_id, tp.member_id, t.amount_cents
		 FROM transactions t
		 JOIN transaction_participants tp ON tp.transaction_id = t.id
		 WHERE t.group_id = ? AND t.cancelled_at IS NULL AND t.type = 'repay'`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("query repayments: %w", err)
	}
	return scanRows(rows, func(r *sql.Rows) (RepaymentRecord, error) {
		var rec RepaymentRecord
		if err := r.Scan(&rec.PayerMemberID, &rec.ToMemberID, &rec.AmountCents); err != nil {
			return rec, fmt.Errorf("scan repayment: %w", err)
		}
		return rec, nil
	})
}

func (s *SQLiteTransactionStore) GetSpending(ctx context.Context, groupID, memberID int64, from, to time.Time) ([]SpendingRow, error) {
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")
	rows, err := s.db.QueryContext(ctx,
		`-- Part A: personal_expense where member is payer
		 SELECT id, group_id, payer_member_id, type, amount_cents,
		        original_amount, original_currency, exchange_rate,
		        description, transaction_date, COALESCE(telegram_message_id, 0),
		        cancelled_at, created_at,
		        amount_cents AS user_share_cents
		 FROM transactions
		 WHERE group_id = ? AND payer_member_id = ? AND cancelled_at IS NULL
		   AND type = 'personal_expense'
		   AND transaction_date >= ? AND transaction_date <= ?

		 UNION ALL

		 -- Part B: split txs where member is payer — net share = total minus what others owe
		 SELECT t.id, t.group_id, t.payer_member_id, t.type, t.amount_cents,
		        t.original_amount, t.original_currency, t.exchange_rate,
		        t.description, t.transaction_date, COALESCE(t.telegram_message_id, 0),
		        t.cancelled_at, t.created_at,
		        (t.amount_cents - COALESCE(SUM(tp.amount_cents), 0)) AS user_share_cents
		 FROM transactions t
		 LEFT JOIN transaction_participants tp ON tp.transaction_id = t.id
		 WHERE t.group_id = ? AND t.payer_member_id = ? AND t.cancelled_at IS NULL
		   AND t.type IN ('split_equal', 'split_except', 'split_partial')
		   AND t.transaction_date >= ? AND t.transaction_date <= ?
		 GROUP BY t.id

		 UNION ALL

		 -- Part C: split txs where member is a non-payer participant
		 SELECT t.id, t.group_id, t.payer_member_id, t.type, t.amount_cents,
		        t.original_amount, t.original_currency, t.exchange_rate,
		        t.description, t.transaction_date, COALESCE(t.telegram_message_id, 0),
		        t.cancelled_at, t.created_at,
		        COALESCE(SUM(tp.amount_cents), 0) AS user_share_cents
		 FROM transactions t
		 JOIN transaction_participants tp ON tp.transaction_id = t.id AND tp.member_id = ?
		 WHERE t.group_id = ? AND t.payer_member_id != ? AND t.cancelled_at IS NULL
		   AND t.type IN ('split_equal', 'split_except', 'split_partial')
		   AND t.transaction_date >= ? AND t.transaction_date <= ?
		 GROUP BY t.id

		 ORDER BY transaction_date DESC`,
		// Part A
		groupID, memberID, fromStr, toStr,
		// Part B
		groupID, memberID, fromStr, toStr,
		// Part C
		memberID, groupID, memberID, fromStr, toStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query spending: %w", err)
	}
	return scanRows(rows, func(r *sql.Rows) (SpendingRow, error) {
		var row SpendingRow
		if err := r.Scan(
			&row.ID, &row.GroupID, &row.PayerMemberID, &row.Type, &row.AmountCents,
			&row.OriginalAmount, &row.OriginalCurrency, &row.ExchangeRate,
			&row.Description, &row.TransactionDate, &row.TelegramMessageID,
			&row.CancelledAt, &row.CreatedAt, &row.UserShareCents,
		); err != nil {
			return row, fmt.Errorf("scan spending row: %w", err)
		}
		return row, nil
	})
}

func (s *SQLiteTransactionStore) GetTotalDebtBetween(ctx context.Context, groupID, fromMemberID, toMemberID int64) (int64, error) {
	// Sum what fromMember owes toMember from participant records (toMember paid, fromMember participated).
	var owed int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(tp.amount_cents), 0)
		 FROM transaction_participants tp
		 JOIN transactions t ON t.id = tp.transaction_id
		 WHERE t.group_id = ? AND t.payer_member_id = ? AND tp.member_id = ?
		   AND t.cancelled_at IS NULL AND t.type != 'repay'`,
		groupID, toMemberID, fromMemberID,
	).Scan(&owed)
	if err != nil {
		return 0, fmt.Errorf("query debt owed: %w", err)
	}

	// Subtract repayments from fromMember to toMember.
	var repaid int64
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(t.amount_cents), 0)
		 FROM transactions t
		 JOIN transaction_participants tp ON tp.transaction_id = t.id
		 WHERE t.group_id = ? AND t.payer_member_id = ? AND tp.member_id = ?
		   AND t.cancelled_at IS NULL AND t.type = 'repay'`,
		groupID, fromMemberID, toMemberID,
	).Scan(&repaid)
	if err != nil {
		return 0, fmt.Errorf("query repayments: %w", err)
	}

	return owed - repaid, nil
}
