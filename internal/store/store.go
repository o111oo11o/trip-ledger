package store

import (
	"context"
	"time"

	"github.com/o111oo11o/trip-ledger/internal/model"
)

// GroupStore manages group persistence.
type GroupStore interface {
	GetOrCreateGroup(ctx context.Context, telegramChatID int64) (*model.Group, error)
	SetDefaultCurrency(ctx context.Context, groupID int64, currency string) error
}

// MemberStore manages member persistence.
type MemberStore interface {
	GetOrCreateMember(ctx context.Context, groupID, telegramUserID int64, username, firstName string) (*model.Member, error)
	GetMemberByUsername(ctx context.Context, groupID int64, username string) (*model.Member, error)
	GetMemberByTelegramID(ctx context.Context, groupID, telegramUserID int64) (*model.Member, error)
	ListMembers(ctx context.Context, groupID int64) ([]model.Member, error)
}

// TransactionStore manages transaction persistence.
type TransactionStore interface {
	CreateTransaction(ctx context.Context, tx *model.Transaction) error
	AddParticipant(ctx context.Context, p *model.TransactionParticipant) error
	AddLineItem(ctx context.Context, li *model.TransactionLineItem) error
	GetTransactionByMessageID(ctx context.Context, groupID, telegramMessageID int64) (*model.Transaction, error)
	CancelTransaction(ctx context.Context, transactionID int64) error
	SetTelegramMessageID(ctx context.Context, transactionID, messageID int64) error

	// GetDebts returns all active participant records for a group, used to compute net debts.
	// Each record represents: participant owes amount to the transaction payer.
	GetActiveParticipants(ctx context.Context, groupID int64) ([]ParticipantWithPayer, error)

	// GetActiveRepayments returns all active repay transactions for a group.
	GetActiveRepayments(ctx context.Context, groupID int64) ([]RepaymentRecord, error)

	// GetSpending returns transactions for a member in a date range.
	GetSpending(ctx context.Context, groupID, memberID int64, from, to time.Time) ([]model.Transaction, error)

	// GetTotalDebtBetween returns the net amount that fromMember owes toMember
	// by summing participant records and repayments.
	GetTotalDebtBetween(ctx context.Context, groupID, fromMemberID, toMemberID int64) (int64, error)
}

// ParticipantWithPayer joins a participant record with its transaction's payer.
type ParticipantWithPayer struct {
	ParticipantMemberID int64
	PayerMemberID       int64
	AmountCents         int64
}

// RepaymentRecord represents a repay transaction.
type RepaymentRecord struct {
	PayerMemberID int64
	ToMemberID    int64
	AmountCents   int64
}
