package model

import "time"

type TransactionType string

const (
	TxPersonalExpense TransactionType = "personal_expense"
	TxSplitEqual      TransactionType = "split_equal"
	TxSplitExcept     TransactionType = "split_except"
	TxSplitPartial    TransactionType = "split_partial"
	TxLend            TransactionType = "lend"
	TxRepay           TransactionType = "repay"
)

type Group struct {
	ID              int64
	TelegramChatID  int64
	DefaultCurrency string
	CreatedAt       time.Time
}

type Member struct {
	ID             int64
	GroupID        int64
	TelegramUserID int64
	Username       string
	FirstName      string
	CreatedAt      time.Time
}

// DisplayName returns the best available display name for a member.
func (m Member) DisplayName() string {
	if m.Username != "" {
		return "@" + m.Username
	}
	if m.FirstName != "" {
		return m.FirstName
	}
	return "Unknown"
}

type Transaction struct {
	ID                int64
	GroupID           int64
	PayerMemberID     int64
	Type              TransactionType
	AmountCents       int64
	OriginalAmount    int64
	OriginalCurrency  string
	ExchangeRate      float64
	Description       string
	TransactionDate   time.Time
	TelegramMessageID int64
	CancelledAt       *time.Time
	CreatedAt         time.Time
}

type TransactionParticipant struct {
	ID            int64
	TransactionID int64
	MemberID      int64
	AmountCents   int64
}

type TransactionLineItem struct {
	ID            int64
	TransactionID int64
	MemberID      int64
	AmountCents   int64
	Description   string
}

// Debt represents how much one member owes another.
type Debt struct {
	FromMember  Member
	ToMember    Member
	AmountCents int64
}

// SpendingEntry represents one line in a spending report.
type SpendingEntry struct {
	Description string
	AmountCents int64
	Date        time.Time
	Type        TransactionType
}

// SpendingSummary holds a user's spending report data.
type SpendingSummary struct {
	Member     Member
	Entries    []SpendingEntry
	TotalCents int64
}
