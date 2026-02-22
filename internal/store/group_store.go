package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/o111oo11o/trip-ledger/internal/model"
)

// SQLiteGroupStore implements GroupStore using SQLite.
type SQLiteGroupStore struct {
	db *DB
}

func NewGroupStore(db *DB) *SQLiteGroupStore {
	return &SQLiteGroupStore{db: db}
}

func (s *SQLiteGroupStore) GetOrCreateGroup(ctx context.Context, telegramChatID int64) (*model.Group, error) {
	var g model.Group
	err := s.db.QueryRowContext(ctx,
		`SELECT id, telegram_chat_id, default_currency, created_at FROM groups WHERE telegram_chat_id = ?`,
		telegramChatID,
	).Scan(&g.ID, &g.TelegramChatID, &g.DefaultCurrency, &g.CreatedAt)

	if err == sql.ErrNoRows {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO groups (telegram_chat_id, default_currency) VALUES (?, 'USD')`,
			telegramChatID,
		)
		if err != nil {
			return nil, fmt.Errorf("insert group: %w", err)
		}
		id, _ := res.LastInsertId()
		return &model.Group{
			ID:              id,
			TelegramChatID:  telegramChatID,
			DefaultCurrency: "USD",
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query group: %w", err)
	}
	return &g, nil
}

func (s *SQLiteGroupStore) SetDefaultCurrency(ctx context.Context, groupID int64, currency string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE groups SET default_currency = ? WHERE id = ?`,
		currency, groupID,
	)
	return err
}
