package service

import (
	"context"
	"testing"
	"time"
)

func setupService(t *testing.T) (*LedgerService, *mockGroupStore, *mockMemberStore, *mockTransactionStore) {
	t.Helper()
	gs := newMockGroupStore()
	ms := newMockMemberStore()
	ts := newMockTransactionStore()
	cc := &mockCurrencyClient{}
	svc := New(gs, ms, ts, cc)
	return svc, gs, ms, ts
}

func TestRecordExpense(t *testing.T) {
	svc, _, _, ts := setupService(t)
	ctx := context.Background()

	group, _ := svc.EnsureGroup(ctx, 100)
	member, _ := svc.EnsureMember(ctx, group.ID, 1, "alice", "Alice")

	tx, err := svc.RecordExpense(ctx, group.ID, member.ID, 4550, "USD", time.Now(), "lunch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.AmountCents != 4550 {
		t.Errorf("expected 4550 cents, got %d", tx.AmountCents)
	}
	if tx.Description != "lunch" {
		t.Errorf("expected description 'lunch', got %q", tx.Description)
	}
	if len(ts.transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(ts.transactions))
	}
	// Personal expense should have no participants.
	if len(ts.participants) != 0 {
		t.Errorf("expected 0 participants, got %d", len(ts.participants))
	}
}

func TestRecordSplitEqual(t *testing.T) {
	svc, _, _, ts := setupService(t)
	ctx := context.Background()

	group, _ := svc.EnsureGroup(ctx, 100)
	alice, _ := svc.EnsureMember(ctx, group.ID, 1, "alice", "Alice")
	_, _ = svc.EnsureMember(ctx, group.ID, 2, "bob", "Bob")
	_, _ = svc.EnsureMember(ctx, group.ID, 3, "charlie", "Charlie")

	tx, err := svc.RecordSplitEqual(ctx, group.ID, alice.ID, 9000, "USD", time.Now(), "dinner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.AmountCents != 9000 {
		t.Errorf("expected 9000 cents, got %d", tx.AmountCents)
	}

	// 3 members total, payer is excluded from participants, so 2 participants.
	if len(ts.participants) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(ts.participants))
	}

	// Each should owe 3000 (9000 / 3).
	for _, p := range ts.participants {
		if p.AmountCents != 3000 {
			t.Errorf("expected participant share 3000, got %d", p.AmountCents)
		}
	}
}

func TestRecordSplitPartial(t *testing.T) {
	svc, _, _, ts := setupService(t)
	ctx := context.Background()

	group, _ := svc.EnsureGroup(ctx, 100)
	alice, _ := svc.EnsureMember(ctx, group.ID, 1, "alice", "Alice")
	bob, _ := svc.EnsureMember(ctx, group.ID, 2, "bob", "Bob")
	_, _ = svc.EnsureMember(ctx, group.ID, 3, "charlie", "Charlie")

	// Alice pays 6000 split between alice and bob.
	_, err := svc.RecordSplitPartial(ctx, group.ID, alice.ID, 6000, "USD", time.Now(), "taxi", []int64{alice.ID, bob.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only bob should be a participant (alice is the payer).
	if len(ts.participants) != 1 {
		t.Fatalf("expected 1 participant, got %d", len(ts.participants))
	}
	if ts.participants[0].MemberID != bob.ID {
		t.Errorf("expected participant to be bob (id %d), got %d", bob.ID, ts.participants[0].MemberID)
	}
	// 6000 / 2 = 3000
	if ts.participants[0].AmountCents != 3000 {
		t.Errorf("expected 3000, got %d", ts.participants[0].AmountCents)
	}
}

func TestRecordLendAndRepay(t *testing.T) {
	svc, _, _, _ := setupService(t)
	ctx := context.Background()

	group, _ := svc.EnsureGroup(ctx, 100)
	alice, _ := svc.EnsureMember(ctx, group.ID, 1, "alice", "Alice")
	bob, _ := svc.EnsureMember(ctx, group.ID, 2, "bob", "Bob")

	// Alice lends 5000 to Bob.
	_, err := svc.RecordLend(ctx, group.ID, alice.ID, bob.ID, 5000, "USD", "loan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check debts.
	debts, err := svc.GetDebts(ctx, group.ID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 1 {
		t.Fatalf("expected 1 debt, got %d", len(debts))
	}
	if debts[0].AmountCents != 5000 {
		t.Errorf("expected debt of 5000, got %d", debts[0].AmountCents)
	}

	// Bob repays 3000.
	_, err = svc.RecordRepay(ctx, group.ID, bob.ID, alice.ID, 3000, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remaining debt should be 2000.
	debts, err = svc.GetDebts(ctx, group.ID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 1 {
		t.Fatalf("expected 1 debt, got %d", len(debts))
	}
	if debts[0].AmountCents != 2000 {
		t.Errorf("expected remaining debt of 2000, got %d", debts[0].AmountCents)
	}
}

func TestRecordRepayFull(t *testing.T) {
	svc, _, _, _ := setupService(t)
	ctx := context.Background()

	group, _ := svc.EnsureGroup(ctx, 100)
	alice, _ := svc.EnsureMember(ctx, group.ID, 1, "alice", "Alice")
	bob, _ := svc.EnsureMember(ctx, group.ID, 2, "bob", "Bob")

	_, err := svc.RecordLend(ctx, group.ID, alice.ID, bob.ID, 5000, "USD", "loan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Full repayment (amount 0).
	_, err = svc.RecordRepay(ctx, group.ID, bob.ID, alice.ID, 0, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	debts, err := svc.GetDebts(ctx, group.ID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 0 {
		t.Errorf("expected no debts after full repayment, got %d", len(debts))
	}
}

func TestCancelTransaction(t *testing.T) {
	svc, _, _, ts := setupService(t)
	ctx := context.Background()

	group, _ := svc.EnsureGroup(ctx, 100)
	alice, _ := svc.EnsureMember(ctx, group.ID, 1, "alice", "Alice")
	bob, _ := svc.EnsureMember(ctx, group.ID, 2, "bob", "Bob")

	_, err := svc.RecordLend(ctx, group.ID, alice.ID, bob.ID, 5000, "USD", "loan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Link to a message ID and cancel.
	_ = svc.SetTransactionMessageID(ctx, ts.transactions[0].ID, 999)
	err = svc.CancelTransaction(ctx, group.ID, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Debts should be empty.
	debts, err := svc.GetDebts(ctx, group.ID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(debts) != 0 {
		t.Errorf("expected no debts after cancellation, got %d", len(debts))
	}
}

func TestSetCurrency(t *testing.T) {
	svc, gs, _, _ := setupService(t)
	ctx := context.Background()

	group, _ := svc.EnsureGroup(ctx, 100)
	if group.DefaultCurrency != "USD" {
		t.Errorf("expected default USD, got %s", group.DefaultCurrency)
	}

	err := svc.SetCurrency(ctx, group.ID, "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gs.groups[group.ID].DefaultCurrency != "EUR" {
		t.Errorf("expected EUR, got %s", gs.groups[group.ID].DefaultCurrency)
	}
}

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		cents    int64
		expected string
	}{
		{0, "0.00"},
		{100, "1.00"},
		{4550, "45.50"},
		{1, "0.01"},
		{99, "0.99"},
		{10050, "100.50"},
	}
	for _, tt := range tests {
		got := FormatMoney(tt.cents)
		if got != tt.expected {
			t.Errorf("FormatMoney(%d) = %q, want %q", tt.cents, got, tt.expected)
		}
	}
}

// Table-driven debt calculation tests.
func TestDebtCalculation(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(svc *LedgerService, groupID int64, aliceID, bobID, charlieID int64)
		filterMember  *int64
		expectedDebts int
		checkFunc     func(t *testing.T, debts []debtCheck)
	}{
		{
			name: "no transactions means no debts",
			setup: func(_ *LedgerService, _ int64, _, _, _ int64) {
				// No transactions.
			},
			expectedDebts: 0,
		},
		{
			name: "single split creates one debt per non-payer member",
			setup: func(svc *LedgerService, groupID int64, aliceID, _, _ int64) {
				_, _ = svc.RecordSplitEqual(context.Background(), groupID, aliceID, 9000, "USD", time.Now(), "dinner")
			},
			expectedDebts: 2,
			checkFunc: func(t *testing.T, debts []debtCheck) {
				for _, d := range debts {
					if d.amount != 3000 {
						t.Errorf("expected debt of 3000, got %d", d.amount)
					}
				}
			},
		},
		{
			name: "lend + partial repay leaves remainder",
			setup: func(svc *LedgerService, groupID int64, aliceID, bobID, _ int64) {
				_, _ = svc.RecordLend(context.Background(), groupID, aliceID, bobID, 10000, "USD", "loan")
				_, _ = svc.RecordRepay(context.Background(), groupID, bobID, aliceID, 3000, "USD")
			},
			expectedDebts: 1,
			checkFunc: func(t *testing.T, debts []debtCheck) {
				if debts[0].amount != 7000 {
					t.Errorf("expected remaining debt of 7000, got %d", debts[0].amount)
				}
			},
		},
		{
			name: "mutual debts get netted",
			setup: func(svc *LedgerService, groupID int64, aliceID, bobID, _ int64) {
				_, _ = svc.RecordLend(context.Background(), groupID, aliceID, bobID, 5000, "USD", "loan1")
				_, _ = svc.RecordLend(context.Background(), groupID, bobID, aliceID, 3000, "USD", "loan2")
			},
			expectedDebts: 1,
			checkFunc: func(t *testing.T, debts []debtCheck) {
				if debts[0].amount != 2000 {
					t.Errorf("expected netted debt of 2000, got %d", debts[0].amount)
				}
			},
		},
		{
			name: "cancelled transaction excluded",
			setup: func(svc *LedgerService, groupID int64, aliceID, bobID, _ int64) {
				tx, _ := svc.RecordLend(context.Background(), groupID, aliceID, bobID, 5000, "USD", "loan")
				_ = svc.SetTransactionMessageID(context.Background(), tx.ID, 42)
				_ = svc.CancelTransaction(context.Background(), groupID, 42)
			},
			expectedDebts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, _ := setupService(t)
			ctx := context.Background()

			group, _ := svc.EnsureGroup(ctx, 100)
			alice, _ := svc.EnsureMember(ctx, group.ID, 1, "alice", "Alice")
			bob, _ := svc.EnsureMember(ctx, group.ID, 2, "bob", "Bob")
			charlie, _ := svc.EnsureMember(ctx, group.ID, 3, "charlie", "Charlie")

			tt.setup(svc, group.ID, alice.ID, bob.ID, charlie.ID)

			debts, err := svc.GetDebts(ctx, group.ID, tt.filterMember)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(debts) != tt.expectedDebts {
				t.Fatalf("expected %d debts, got %d", tt.expectedDebts, len(debts))
			}
			if tt.checkFunc != nil {
				var checks []debtCheck
				for _, d := range debts {
					checks = append(checks, debtCheck{
						fromID: d.FromMember.ID,
						toID:   d.ToMember.ID,
						amount: d.AmountCents,
					})
				}
				tt.checkFunc(t, checks)
			}
		})
	}
}

type debtCheck struct {
	fromID int64
	toID   int64
	amount int64
}
