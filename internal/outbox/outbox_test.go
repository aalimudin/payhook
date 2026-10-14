package outbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/payhook/payhook/internal/provider"
	"github.com/payhook/payhook/internal/store"
	"github.com/payhook/payhook/internal/testdb"
)

func setup(t *testing.T, url string, max int) (*Worker, *Worker) {
	st := store.Store{DB: testdb.Open(t)}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Record(context.Background(), "stripe", provider.Event{ID: "e1", OrderRef: "o1", Status: "paid", Amount: 1}); err != nil {
		t.Fatal(err)
	}
	w := &Worker{DB: st.DB, URL: url, MaxAttempts: max, Batch: 10, Poll: time.Millisecond, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
	return w, w
}

func drain(t *testing.T, w *Worker) {
	for i := 0; i < 30; i++ {
		if _, err := w.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(8 * time.Millisecond)
	}
}

func status(t *testing.T, w *Worker) (s string, attempts int) {
	if err := w.DB.QueryRow(`SELECT status, attempts FROM outbox`).Scan(&s, &attempts); err != nil {
		t.Fatal(err)
	}
	return
}

func TestFlakyDownstreamDeliveredOnce(t *testing.T) {
	var calls, ok atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "e1" {
			t.Error("missing Idempotency-Key")
		}
		if calls.Add(1) <= 3 {
			rw.WriteHeader(500)
			return
		}
		ok.Add(1)
	}))
	defer srv.Close()
	w, _ := setup(t, srv.URL, 8)
	drain(t, w)
	if s, a := status(t, w); s != "delivered" || a != 4 {
		t.Fatalf("status=%s attempts=%d", s, a)
	}
	if ok.Load() != 1 {
		t.Fatalf("successful deliveries = %d", ok.Load())
	}
}

func TestAlwaysFailingGoesDead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(503) }))
	defer srv.Close()
	w, _ := setup(t, srv.URL, 3)
	drain(t, w)
	if s, a := status(t, w); s != "dead" || a != 3 {
		t.Fatalf("status=%s attempts=%d", s, a)
	}
}
