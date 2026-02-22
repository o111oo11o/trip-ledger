package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/o111oo11o/trip-ledger/internal/model"
	"github.com/o111oo11o/trip-ledger/internal/store"
	"github.com/o111oo11o/trip-ledger/pkg/currency"
)

// LedgerService contains all business logic for expense tracking.
type LedgerService struct {
	groups       store.GroupStore
	members      store.MemberStore
	transactions store.TransactionStore
	currency     currency.Client
}

// New creates a new LedgerService.
func New(gs store.GroupStore, ms store.MemberStore, ts store.TransactionStore, cc currency.Client) *LedgerService {
	return &LedgerService{
		groups:       gs,
		members:      ms,
		transactions: ts,
		currency:     cc,
	}
}

// EnsureGroup returns or creates the group for the given Telegram chat.
func (s *LedgerService) EnsureGroup(ctx context.Context, chatID int64) (*model.Group, error) {
	return s.groups.GetOrCreateGroup(ctx, chatID)
}

// EnsureMember returns or creates the member for the given Telegram user in a group.
func (s *LedgerService) EnsureMember(ctx context.Context, groupID, telegramUserID int64, username, firstName string) (*model.Member, error) {
	return s.members.GetOrCreateMember(ctx, groupID, telegramUserID, username, firstName)
}

// ResolveMember looks up a member by @username in a group.
func (s *LedgerService) ResolveMember(ctx context.Context, groupID int64, username string) (*model.Member, error) {
	return s.members.GetMemberByUsername(ctx, groupID, username)
}

// SetCurrency sets the default currency for a group.
func (s *LedgerService) SetCurrency(ctx context.Context, groupID int64, currencyCode string) error {
	if !currency.IsValidCurrency(currencyCode) {
		return fmt.Errorf("invalid currency code: %s", currencyCode)
	}
	// Validate currency exists by trying to get rate.
	if _, err := s.currency.Rate(currencyCode); err != nil {
		return fmt.Errorf("unsupported currency: %s", currencyCode)
	}
	return s.groups.SetDefaultCurrency(ctx, groupID, currencyCode)
}

// newTx converts amountCents from curr to USD, populates and returns a *model.Transaction.
// The caller must call CreateTransaction after any further field adjustments.
func (s *LedgerService) newTx(groupID, payerID, amountCents int64, curr string, txType model.TransactionType, date time.Time, description string) (*model.Transaction, error) {
	usdCents, rate, err := s.currency.Convert(amountCents, curr)
	if err != nil {
		return nil, err
	}
	return &model.Transaction{
		GroupID:          groupID,
		PayerMemberID:    payerID,
		Type:             txType,
		AmountCents:      usdCents,
		OriginalAmount:   amountCents,
		OriginalCurrency: curr,
		ExchangeRate:     rate,
		Description:      description,
		TransactionDate:  date,
	}, nil
}

// RecordExpense records a personal expense.
func (s *LedgerService) RecordExpense(ctx context.Context, groupID int64, payerID int64, amountCents int64, curr string, date time.Time, description string) (*model.Transaction, error) {
	tx, err := s.newTx(groupID, payerID, amountCents, curr, model.TxPersonalExpense, date, description)
	if err != nil {
		return nil, err
	}
	if err := s.transactions.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// RecordSplitEqual records an expense split equally among all group members.
func (s *LedgerService) RecordSplitEqual(ctx context.Context, groupID int64, payerID int64, amountCents int64, curr string, date time.Time, description string) (*model.Transaction, error) {
	members, err := s.members.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("no members in group")
	}

	tx, err := s.newTx(groupID, payerID, amountCents, curr, model.TxSplitEqual, date, description)
	if err != nil {
		return nil, err
	}
	if err := s.transactions.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}

	shareEach := tx.AmountCents / int64(len(members))
	for _, m := range members {
		if m.ID == payerID {
			continue // payer doesn't owe themselves
		}
		p := &model.TransactionParticipant{
			TransactionID: tx.ID,
			MemberID:      m.ID,
			AmountCents:   shareEach,
		}
		if err := s.transactions.AddParticipant(ctx, p); err != nil {
			return nil, err
		}
	}
	return tx, nil
}

// LineItem holds a personal line item for splitexcept.
type LineItem struct {
	MemberID    int64
	AmountCents int64
	Description string
}

// RecordSplitExcept records an expense split equally after subtracting personal line items.
func (s *LedgerService) RecordSplitExcept(ctx context.Context, groupID int64, payerID int64, totalAmountCents int64, curr string, description string, lineItems []LineItem) (*model.Transaction, error) {
	members, err := s.members.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("no members in group")
	}

	tx, err := s.newTx(groupID, payerID, totalAmountCents, curr, model.TxSplitExcept, time.Now(), description)
	if err != nil {
		return nil, err
	}
	if err := s.transactions.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}

	// Convert and save line items; subtract from total.
	var personalTotal int64
	for _, li := range lineItems {
		liUSD := int64(math.Round(float64(li.AmountCents) * tx.ExchangeRate))
		personalTotal += liUSD
		item := &model.TransactionLineItem{
			TransactionID: tx.ID,
			MemberID:      li.MemberID,
			AmountCents:   liUSD,
			Description:   li.Description,
		}
		if err := s.transactions.AddLineItem(ctx, item); err != nil {
			return nil, err
		}
		// The line item owner owes the payer this amount (if they aren't the payer).
		if li.MemberID != payerID {
			p := &model.TransactionParticipant{
				TransactionID: tx.ID,
				MemberID:      li.MemberID,
				AmountCents:   liUSD,
			}
			if err := s.transactions.AddParticipant(ctx, p); err != nil {
				return nil, err
			}
		}
	}

	// Remaining amount is split equally among all members.
	remaining := tx.AmountCents - personalTotal
	if remaining < 0 {
		remaining = 0
	}
	shareEach := remaining / int64(len(members))
	for _, m := range members {
		if m.ID == payerID {
			continue
		}
		p := &model.TransactionParticipant{
			TransactionID: tx.ID,
			MemberID:      m.ID,
			AmountCents:   shareEach,
		}
		if err := s.transactions.AddParticipant(ctx, p); err != nil {
			return nil, err
		}
	}
	return tx, nil
}

// RecordSplitPartial records an expense split equally among a subset of members.
func (s *LedgerService) RecordSplitPartial(ctx context.Context, groupID int64, payerID int64, amountCents int64, curr string, date time.Time, description string, participantIDs []int64) (*model.Transaction, error) {
	if len(participantIDs) == 0 {
		return nil, fmt.Errorf("at least one participant required")
	}

	tx, err := s.newTx(groupID, payerID, amountCents, curr, model.TxSplitPartial, date, description)
	if err != nil {
		return nil, err
	}
	if err := s.transactions.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}

	shareEach := tx.AmountCents / int64(len(participantIDs))
	for _, mid := range participantIDs {
		if mid == payerID {
			continue
		}
		p := &model.TransactionParticipant{
			TransactionID: tx.ID,
			MemberID:      mid,
			AmountCents:   shareEach,
		}
		if err := s.transactions.AddParticipant(ctx, p); err != nil {
			return nil, err
		}
	}
	return tx, nil
}

// RecordLend records a loan from one member to another.
func (s *LedgerService) RecordLend(ctx context.Context, groupID int64, fromID, toID int64, amountCents int64, curr string, description string) (*model.Transaction, error) {
	tx, err := s.newTx(groupID, fromID, amountCents, curr, model.TxLend, time.Now(), description)
	if err != nil {
		return nil, err
	}
	if err := s.transactions.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}
	p := &model.TransactionParticipant{
		TransactionID: tx.ID,
		MemberID:      toID,
		AmountCents:   tx.AmountCents,
	}
	return tx, s.transactions.AddParticipant(ctx, p)
}

// RecordRepay records a repayment. If amountCents is 0, repays full outstanding debt.
func (s *LedgerService) RecordRepay(ctx context.Context, groupID int64, fromID, toID int64, amountCents int64, curr string) (*model.Transaction, error) {
	if amountCents == 0 {
		// Full repayment: find how much fromMember owes toMember.
		debt, err := s.transactions.GetTotalDebtBetween(ctx, groupID, fromID, toID)
		if err != nil {
			return nil, err
		}
		if debt <= 0 {
			return nil, fmt.Errorf("no outstanding debt to repay")
		}
		amountCents = debt
		curr = "USD" // already in USD cents
	}

	tx, err := s.newTx(groupID, fromID, amountCents, curr, model.TxRepay, time.Now(), "")
	if err != nil {
		return nil, err
	}
	if err := s.transactions.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}

	// Participant is the person being repaid (the lender).
	p := &model.TransactionParticipant{
		TransactionID: tx.ID,
		MemberID:      toID,
		AmountCents:   tx.AmountCents,
	}
	return tx, s.transactions.AddParticipant(ctx, p)
}

// SetTransactionMessageID sets the Telegram message ID on a transaction for /cancel support.
func (s *LedgerService) SetTransactionMessageID(ctx context.Context, txID, messageID int64) error {
	return s.transactions.SetTelegramMessageID(ctx, txID, messageID)
}

// CancelTransaction soft-deletes a transaction by its linked Telegram message ID.
func (s *LedgerService) CancelTransaction(ctx context.Context, groupID, telegramMessageID int64) error {
	tx, err := s.transactions.GetTransactionByMessageID(ctx, groupID, telegramMessageID)
	if err != nil {
		return err
	}
	return s.transactions.CancelTransaction(ctx, tx.ID)
}

// GetDebts computes the current debt summary for a group.
// Returns a list of simplified debts after netting.
func (s *LedgerService) GetDebts(ctx context.Context, groupID int64, filterMemberID *int64) ([]model.Debt, error) {
	participants, err := s.transactions.GetActiveParticipants(ctx, groupID)
	if err != nil {
		return nil, err
	}
	repayments, err := s.transactions.GetActiveRepayments(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members, err := s.members.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}

	memberMap := make(map[int64]model.Member)
	for _, m := range members {
		memberMap[m.ID] = m
	}

	// Net balance: positive = is owed money, negative = owes money.
	// debt[A][B] = how much A owes B.
	type pair struct{ from, to int64 }
	debts := make(map[pair]int64)

	for _, p := range participants {
		key := pair{from: p.ParticipantMemberID, to: p.PayerMemberID}
		debts[key] += p.AmountCents
	}
	for _, r := range repayments {
		// Repayment: payer repaid to participant.
		key := pair{from: r.PayerMemberID, to: r.ToMemberID}
		debts[key] -= r.AmountCents
	}

	// Simplify: net out A->B vs B->A.
	type orderedPair struct{ a, b int64 }
	netted := make(map[orderedPair]int64)
	for k, v := range debts {
		var op orderedPair
		if k.from < k.to {
			op = orderedPair{k.from, k.to}
			netted[op] += v
		} else {
			op = orderedPair{k.to, k.from}
			netted[op] -= v
		}
	}

	var result []model.Debt
	for op, amount := range netted {
		if amount == 0 {
			continue
		}
		var d model.Debt
		if amount > 0 {
			d = model.Debt{
				FromMember:  memberMap[op.a],
				ToMember:    memberMap[op.b],
				AmountCents: amount,
			}
		} else {
			d = model.Debt{
				FromMember:  memberMap[op.b],
				ToMember:    memberMap[op.a],
				AmountCents: -amount,
			}
		}
		if filterMemberID != nil && d.FromMember.ID != *filterMemberID && d.ToMember.ID != *filterMemberID {
			continue
		}
		result = append(result, d)
	}
	return result, nil
}

// GetSpending returns a spending summary for a member in a date range.
// memberID is the internal (store) member ID.
func (s *LedgerService) GetSpending(ctx context.Context, groupID, memberID int64, from, to time.Time) (*model.SpendingSummary, error) {
	txs, err := s.transactions.GetSpending(ctx, groupID, memberID, from, to)
	if err != nil {
		return nil, err
	}

	summary := &model.SpendingSummary{Member: model.Member{ID: memberID}}
	for _, tx := range txs {
		summary.Entries = append(summary.Entries, model.SpendingEntry{
			Description: tx.Description,
			AmountCents: tx.AmountCents,
			Date:        tx.TransactionDate,
			Type:        tx.Type,
		})
		summary.TotalCents += tx.AmountCents
	}
	return summary, nil
}

// ConvertFromUSD converts USD cents to the target currency for display.
func (s *LedgerService) ConvertFromUSD(usdCents int64, toCurrency string) (int64, error) {
	return s.currency.ConvertFromUSD(usdCents, toCurrency)
}

// FormatMoney formats cents as a human-readable string like "45.50".
func FormatMoney(cents int64) string {
	whole := cents / 100
	frac := cents % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}
