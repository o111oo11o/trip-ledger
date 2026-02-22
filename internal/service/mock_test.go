package service

import (
	"context"
	"fmt"
	"time"

	"github.com/o111oo11o/trip-ledger/internal/model"
	"github.com/o111oo11o/trip-ledger/internal/store"
)

// mockGroupStore implements store.GroupStore for testing.
type mockGroupStore struct {
	groups map[int64]*model.Group
	nextID int64
}

func newMockGroupStore() *mockGroupStore {
	return &mockGroupStore{groups: make(map[int64]*model.Group), nextID: 1}
}

func (m *mockGroupStore) GetOrCreateGroup(_ context.Context, telegramChatID int64) (*model.Group, error) {
	for _, g := range m.groups {
		if g.TelegramChatID == telegramChatID {
			return g, nil
		}
	}
	g := &model.Group{
		ID:              m.nextID,
		TelegramChatID:  telegramChatID,
		DefaultCurrency: "USD",
		CreatedAt:       time.Now(),
	}
	m.nextID++
	m.groups[g.ID] = g
	return g, nil
}

func (m *mockGroupStore) SetDefaultCurrency(_ context.Context, groupID int64, currency string) error {
	g, ok := m.groups[groupID]
	if !ok {
		return fmt.Errorf("group not found")
	}
	g.DefaultCurrency = currency
	return nil
}

// mockMemberStore implements store.MemberStore for testing.
type mockMemberStore struct {
	members map[int64]*model.Member
	nextID  int64
}

func newMockMemberStore() *mockMemberStore {
	return &mockMemberStore{members: make(map[int64]*model.Member), nextID: 1}
}

func (m *mockMemberStore) GetOrCreateMember(_ context.Context, groupID, telegramUserID int64, username, firstName string) (*model.Member, error) {
	for _, mem := range m.members {
		if mem.GroupID == groupID && mem.TelegramUserID == telegramUserID {
			return mem, nil
		}
	}
	mem := &model.Member{
		ID:             m.nextID,
		GroupID:        groupID,
		TelegramUserID: telegramUserID,
		Username:       username,
		FirstName:      firstName,
		CreatedAt:      time.Now(),
	}
	m.nextID++
	m.members[mem.ID] = mem
	return mem, nil
}

func (m *mockMemberStore) GetMemberByUsername(_ context.Context, groupID int64, username string) (*model.Member, error) {
	for _, mem := range m.members {
		if mem.GroupID == groupID && mem.Username == username {
			return mem, nil
		}
	}
	return nil, fmt.Errorf("member @%s not found", username)
}

func (m *mockMemberStore) GetMemberByTelegramID(_ context.Context, groupID, telegramUserID int64) (*model.Member, error) {
	for _, mem := range m.members {
		if mem.GroupID == groupID && mem.TelegramUserID == telegramUserID {
			return mem, nil
		}
	}
	return nil, fmt.Errorf("member not found")
}

func (m *mockMemberStore) ListMembers(_ context.Context, groupID int64) ([]model.Member, error) {
	var result []model.Member
	for _, mem := range m.members {
		if mem.GroupID == groupID {
			result = append(result, *mem)
		}
	}
	return result, nil
}

// mockTransactionStore implements store.TransactionStore for testing.
type mockTransactionStore struct {
	transactions []model.Transaction
	participants []model.TransactionParticipant
	lineItems    []model.TransactionLineItem
	nextTxID     int64
	nextPartID   int64
	nextLiID     int64
}

func newMockTransactionStore() *mockTransactionStore {
	return &mockTransactionStore{nextTxID: 1, nextPartID: 1, nextLiID: 1}
}

func (m *mockTransactionStore) CreateTransaction(_ context.Context, tx *model.Transaction) error {
	tx.ID = m.nextTxID
	m.nextTxID++
	tx.CreatedAt = time.Now()
	m.transactions = append(m.transactions, *tx)
	return nil
}

func (m *mockTransactionStore) AddParticipant(_ context.Context, p *model.TransactionParticipant) error {
	p.ID = m.nextPartID
	m.nextPartID++
	m.participants = append(m.participants, *p)
	return nil
}

func (m *mockTransactionStore) AddLineItem(_ context.Context, li *model.TransactionLineItem) error {
	li.ID = m.nextLiID
	m.nextLiID++
	m.lineItems = append(m.lineItems, *li)
	return nil
}

func (m *mockTransactionStore) GetTransactionByMessageID(_ context.Context, groupID, telegramMessageID int64) (*model.Transaction, error) {
	for _, tx := range m.transactions {
		if tx.GroupID == groupID && tx.TelegramMessageID == telegramMessageID && tx.CancelledAt == nil {
			return &tx, nil
		}
	}
	return nil, fmt.Errorf("transaction not found")
}

func (m *mockTransactionStore) CancelTransaction(_ context.Context, transactionID int64) error {
	now := time.Now()
	for i, tx := range m.transactions {
		if tx.ID == transactionID {
			m.transactions[i].CancelledAt = &now
			return nil
		}
	}
	return fmt.Errorf("transaction not found")
}

func (m *mockTransactionStore) SetTelegramMessageID(_ context.Context, transactionID, messageID int64) error {
	for i, tx := range m.transactions {
		if tx.ID == transactionID {
			m.transactions[i].TelegramMessageID = messageID
			return nil
		}
	}
	return fmt.Errorf("transaction not found")
}

// txIndex builds an id→transaction index over the mock's transaction slice.
func (m *mockTransactionStore) txIndex() map[int64]*model.Transaction {
	idx := make(map[int64]*model.Transaction, len(m.transactions))
	for i := range m.transactions {
		idx[m.transactions[i].ID] = &m.transactions[i]
	}
	return idx
}

func (m *mockTransactionStore) GetActiveParticipants(_ context.Context, groupID int64) ([]store.ParticipantWithPayer, error) {
	txMap := m.txIndex()
	var result []store.ParticipantWithPayer
	for _, p := range m.participants {
		tx, ok := txMap[p.TransactionID]
		if !ok || tx.GroupID != groupID || tx.CancelledAt != nil || tx.Type == model.TxRepay {
			continue
		}
		result = append(result, store.ParticipantWithPayer{
			ParticipantMemberID: p.MemberID,
			PayerMemberID:       tx.PayerMemberID,
			AmountCents:         p.AmountCents,
		})
	}
	return result, nil
}

func (m *mockTransactionStore) GetActiveRepayments(_ context.Context, groupID int64) ([]store.RepaymentRecord, error) {
	txMap := m.txIndex()
	var result []store.RepaymentRecord
	for _, p := range m.participants {
		tx, ok := txMap[p.TransactionID]
		if !ok || tx.GroupID != groupID || tx.CancelledAt != nil || tx.Type != model.TxRepay {
			continue
		}
		result = append(result, store.RepaymentRecord{
			PayerMemberID: tx.PayerMemberID,
			ToMemberID:    p.MemberID,
			AmountCents:   tx.AmountCents,
		})
	}
	return result, nil
}

func (m *mockTransactionStore) GetSpending(_ context.Context, groupID, memberID int64, from, to time.Time) ([]model.Transaction, error) {
	var result []model.Transaction
	for _, tx := range m.transactions {
		if tx.GroupID == groupID && tx.PayerMemberID == memberID && tx.CancelledAt == nil &&
			!tx.TransactionDate.Before(from) && !tx.TransactionDate.After(to) {
			result = append(result, tx)
		}
	}
	return result, nil
}

func (m *mockTransactionStore) GetTotalDebtBetween(_ context.Context, groupID, fromMemberID, toMemberID int64) (int64, error) {
	txMap := m.txIndex()

	var owed, repaid int64
	for _, p := range m.participants {
		tx, ok := txMap[p.TransactionID]
		if !ok || tx.GroupID != groupID || tx.CancelledAt != nil {
			continue
		}
		switch tx.Type {
		case model.TxRepay:
			if tx.PayerMemberID == fromMemberID && p.MemberID == toMemberID {
				repaid += tx.AmountCents
			}
		default:
			if tx.PayerMemberID == toMemberID && p.MemberID == fromMemberID {
				owed += p.AmountCents
			}
		}
	}
	return owed - repaid, nil
}

// mockCurrencyClient implements currency.Client for testing (1:1 rates).
type mockCurrencyClient struct{}

func (m *mockCurrencyClient) Rate(_ string) (float64, error) { return 1.0, nil }
func (m *mockCurrencyClient) Convert(amountCents int64, _ string) (int64, float64, error) {
	return amountCents, 1.0, nil
}
func (m *mockCurrencyClient) ConvertFromUSD(usdCents int64, _ string) (int64, error) {
	return usdCents, nil
}
