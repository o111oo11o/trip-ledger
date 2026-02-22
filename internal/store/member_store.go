package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/o111oo11o/trip-ledger/internal/model"
)

// SQLiteMemberStore implements MemberStore using SQLite.
type SQLiteMemberStore struct {
	db *DB
}

func NewMemberStore(db *DB) *SQLiteMemberStore {
	return &SQLiteMemberStore{db: db}
}

func (s *SQLiteMemberStore) GetOrCreateMember(ctx context.Context, groupID, telegramUserID int64, username, firstName string) (*model.Member, error) {
	var m model.Member
	err := s.db.QueryRowContext(ctx,
		`SELECT id, group_id, telegram_user_id, username, first_name, created_at
		 FROM members WHERE group_id = ? AND telegram_user_id = ?`,
		groupID, telegramUserID,
	).Scan(&m.ID, &m.GroupID, &m.TelegramUserID, &m.Username, &m.FirstName, &m.CreatedAt)

	if err == sql.ErrNoRows {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO members (group_id, telegram_user_id, username, first_name) VALUES (?, ?, ?, ?)`,
			groupID, telegramUserID, username, firstName,
		)
		if err != nil {
			return nil, fmt.Errorf("insert member: %w", err)
		}
		id, _ := res.LastInsertId()
		m = model.Member{
			ID: id, GroupID: groupID, TelegramUserID: telegramUserID,
			Username: username, FirstName: firstName,
		}
		return &m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query member: %w", err)
	}

	// Update username/firstName if changed.
	if m.Username != username || m.FirstName != firstName {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE members SET username = ?, first_name = ? WHERE id = ?`,
			username, firstName, m.ID,
		)
		m.Username = username
		m.FirstName = firstName
	}
	return &m, nil
}

// scanOneMember scans a single member row, returning the member or a wrapped error.
// notFoundMsg is returned verbatim when no row exists; queryDesc labels the query in error wrapping.
func scanOneMember(row *sql.Row, notFoundMsg, queryDesc string) (*model.Member, error) {
	var m model.Member
	err := row.Scan(&m.ID, &m.GroupID, &m.TelegramUserID, &m.Username, &m.FirstName, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%s", notFoundMsg)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", queryDesc, err)
	}
	return &m, nil
}

func (s *SQLiteMemberStore) GetMemberByUsername(ctx context.Context, groupID int64, username string) (*model.Member, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, group_id, telegram_user_id, username, first_name, created_at
		 FROM members WHERE group_id = ? AND username = ?`,
		groupID, username,
	)
	return scanOneMember(row, fmt.Sprintf("member @%s not found in this group", username), "query member by username")
}

func (s *SQLiteMemberStore) GetMemberByTelegramID(ctx context.Context, groupID, telegramUserID int64) (*model.Member, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, group_id, telegram_user_id, username, first_name, created_at
		 FROM members WHERE group_id = ? AND telegram_user_id = ?`,
		groupID, telegramUserID,
	)
	return scanOneMember(row, "member not found", "query member by telegram id")
}

func (s *SQLiteMemberStore) ListMembers(ctx context.Context, groupID int64) ([]model.Member, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, group_id, telegram_user_id, username, first_name, created_at
		 FROM members WHERE group_id = ? ORDER BY id`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var members []model.Member
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.ID, &m.GroupID, &m.TelegramUserID, &m.Username, &m.FirstName, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}
