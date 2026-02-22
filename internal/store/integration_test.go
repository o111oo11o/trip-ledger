package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/o111oo11o/trip-ledger/internal/model"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	migrationSQL, err := os.ReadFile("../../migrations/001_init.up.sql")
	if err != nil {
		t.Fatalf("failed to read migration: %v", err)
	}
	if err := db.Migrate(string(migrationSQL)); err != nil {
		t.Fatalf("failed to run migration: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	})
	return db
}

func TestIntegrationFullWorkflow(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	gs := NewGroupStore(db)
	ms := NewMemberStore(db)
	ts := NewTransactionStore(db)

	// Create group.
	group, err := gs.GetOrCreateGroup(ctx, 12345)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if group.DefaultCurrency != "USD" {
		t.Errorf("expected USD, got %s", group.DefaultCurrency)
	}

	// Idempotent.
	group2, err := gs.GetOrCreateGroup(ctx, 12345)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if group2.ID != group.ID {
		t.Errorf("expected same group ID")
	}

	// Create members.
	alice, err := ms.GetOrCreateMember(ctx, group.ID, 1, "alice", "Alice")
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := ms.GetOrCreateMember(ctx, group.ID, 2, "bob", "Bob")
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	charlie, err := ms.GetOrCreateMember(ctx, group.ID, 3, "charlie", "Charlie")
	if err != nil {
		t.Fatalf("create charlie: %v", err)
	}

	// List members.
	members, err := ms.ListMembers(ctx, group.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	// Lookup by username.
	found, err := ms.GetMemberByUsername(ctx, group.ID, "bob")
	if err != nil {
		t.Fatalf("get by username: %v", err)
	}
	if found.ID != bob.ID {
		t.Errorf("expected bob's ID")
	}

	// Create a split-equal transaction: Alice pays 9000 (90.00 USD).
	tx := &model.Transaction{
		GroupID:          group.ID,
		PayerMemberID:    alice.ID,
		Type:             model.TxSplitEqual,
		AmountCents:      9000,
		OriginalAmount:   9000,
		OriginalCurrency: "USD",
		ExchangeRate:     1.0,
		Description:      "dinner",
		TransactionDate:  time.Now(),
	}
	if err := ts.CreateTransaction(ctx, tx); err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	if tx.ID == 0 {
		t.Error("expected non-zero transaction ID")
	}

	// Add participants: bob and charlie each owe 3000.
	for _, m := range []*model.Member{bob, charlie} {
		p := &model.TransactionParticipant{
			TransactionID: tx.ID,
			MemberID:      m.ID,
			AmountCents:   3000,
		}
		if err := ts.AddParticipant(ctx, p); err != nil {
			t.Fatalf("add participant: %v", err)
		}
	}

	// Link message ID.
	if err := ts.SetTelegramMessageID(ctx, tx.ID, 999); err != nil {
		t.Fatalf("set message id: %v", err)
	}

	// Verify message ID lookup.
	found2, err := ts.GetTransactionByMessageID(ctx, group.ID, 999)
	if err != nil {
		t.Fatalf("get by message id: %v", err)
	}
	if found2.ID != tx.ID {
		t.Errorf("expected tx ID %d, got %d", tx.ID, found2.ID)
	}

	// Check active participants.
	participants, err := ts.GetActiveParticipants(ctx, group.ID)
	if err != nil {
		t.Fatalf("get active participants: %v", err)
	}
	if len(participants) != 2 {
		t.Fatalf("expected 2 active participants, got %d", len(participants))
	}

	// Check debt between bob and alice.
	debt, err := ts.GetTotalDebtBetween(ctx, group.ID, bob.ID, alice.ID)
	if err != nil {
		t.Fatalf("get debt: %v", err)
	}
	if debt != 3000 {
		t.Errorf("expected debt of 3000, got %d", debt)
	}

	// Record a repayment: bob repays 1000 to alice.
	repayTx := &model.Transaction{
		GroupID:          group.ID,
		PayerMemberID:    bob.ID,
		Type:             model.TxRepay,
		AmountCents:      1000,
		OriginalAmount:   1000,
		OriginalCurrency: "USD",
		ExchangeRate:     1.0,
		TransactionDate:  time.Now(),
	}
	if err := ts.CreateTransaction(ctx, repayTx); err != nil {
		t.Fatalf("create repay: %v", err)
	}
	repayP := &model.TransactionParticipant{
		TransactionID: repayTx.ID,
		MemberID:      alice.ID,
		AmountCents:   1000,
	}
	if err := ts.AddParticipant(ctx, repayP); err != nil {
		t.Fatalf("add repay participant: %v", err)
	}

	// Remaining debt should be 2000.
	debt, err = ts.GetTotalDebtBetween(ctx, group.ID, bob.ID, alice.ID)
	if err != nil {
		t.Fatalf("get debt after repay: %v", err)
	}
	if debt != 2000 {
		t.Errorf("expected remaining debt of 2000, got %d", debt)
	}

	// Cancel the original transaction.
	if err := ts.CancelTransaction(ctx, tx.ID); err != nil {
		t.Fatalf("cancel transaction: %v", err)
	}

	// After cancellation, bob's debt to alice from participants should be 0,
	// but repayment subtracts 1000, so net is -1000 (alice owes bob).
	debt, err = ts.GetTotalDebtBetween(ctx, group.ID, bob.ID, alice.ID)
	if err != nil {
		t.Fatalf("get debt after cancel: %v", err)
	}
	if debt != -1000 {
		t.Errorf("expected debt of -1000 after cancel, got %d", debt)
	}

	// Set currency.
	if err := gs.SetDefaultCurrency(ctx, group.ID, "EUR"); err != nil {
		t.Fatalf("set currency: %v", err)
	}
	updated, _ := gs.GetOrCreateGroup(ctx, 12345)
	if updated.DefaultCurrency != "EUR" {
		t.Errorf("expected EUR, got %s", updated.DefaultCurrency)
	}

	// Spending query.
	from := time.Now().AddDate(0, 0, -1)
	to := time.Now().AddDate(0, 0, 1)
	spending, err := ts.GetSpending(ctx, group.ID, alice.ID, from, to)
	if err != nil {
		t.Fatalf("get spending: %v", err)
	}
	// The original split was cancelled, so 0 spending results.
	if len(spending) != 0 {
		t.Errorf("expected 0 spending after cancel, got %d", len(spending))
	}

	// Test member username update.
	_, err = ms.GetOrCreateMember(ctx, group.ID, 1, "alice_new", "Alice N")
	if err != nil {
		t.Fatalf("update alice: %v", err)
	}
	aliceUpdated, _ := ms.GetMemberByUsername(ctx, group.ID, "alice_new")
	if aliceUpdated.FirstName != "Alice N" {
		t.Errorf("expected updated first name")
	}

	_ = charlie // used in participant creation
}
