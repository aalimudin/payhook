package store

import (
	"context"
	"sync"
	"testing"

	"github.com/payhook/payhook/internal/provider"
	"github.com/payhook/payhook/internal/testdb"
)

func setup(t *testing.T) Store {
	st := Store{DB: testdb.Open(t)}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

func count(t *testing.T, st Store, q string) (n int) {
	t.Helper()
	if err := st.DB.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return
}

func TestConcurrentDuplicatesYieldOneOutboxRow(t *testing.T) {
	st := setup(t)
	ev := provider.Event{ID: "e1", OrderRef: "o1", Status: "paid", Amount: 100}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.Record(context.Background(), "stripe", ev); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := count(t, st, `SELECT count(*) FROM outbox`); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
	if n := count(t, st, `SELECT count(*) FROM events`); n != 1 {
		t.Fatalf("events = %d, want 1", n)
	}
}

func TestOutOfOrderAndAmountMismatch(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	rec := func(id, status string, amt int64) string {
		out, err := st.Record(ctx, "midtrans", provider.Event{ID: id, OrderRef: "o1", Status: status, Amount: amt})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := rec("a", "pending", 100); got != Processed {
		t.Fatalf("pending: %s", got)
	}
	if got := rec("b", "paid", 50); got != RejectedAmount {
		t.Fatalf("underpaid: %s", got)
	}
	if got := rec("c", "paid", 100); got != Processed {
		t.Fatalf("paid: %s", got)
	}
	if got := rec("d", "pending", 100); got != RejectedTransition {
		t.Fatalf("late pending: %s", got)
	}
	var status string
	st.DB.QueryRow(`SELECT status FROM payments WHERE order_ref='o1'`).Scan(&status)
	if status != "paid" {
		t.Fatalf("status = %s, want paid", status)
	}
	if n := count(t, st, `SELECT count(*) FROM outbox`); n != 2 { // pending + paid only
		t.Fatalf("outbox rows = %d, want 2", n)
	}
}

func TestZeroAmountPendingThenPaid(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	for i, c := range []struct {
		id, status string
		amt        int64
		want       string
	}{
		{"a", "pending", 0, Processed}, // SNAP pending carries no amount
		{"b", "paid", 100, Processed},
		{"c", "refunded", 0, Processed},
	} {
		got, err := st.Record(ctx, "midtrans-snap", provider.Event{ID: c.id, OrderRef: "o1", Status: c.status, Amount: c.amt})
		if err != nil || got != c.want {
			t.Fatalf("step %d: %s %v", i, got, err)
		}
	}
	if got, _ := st.Record(ctx, "midtrans-snap", provider.Event{ID: "z", OrderRef: "o2", Status: "paid", Amount: 0}); got != RejectedAmount {
		t.Fatalf("zero-amount paid: %s", got)
	}
}
