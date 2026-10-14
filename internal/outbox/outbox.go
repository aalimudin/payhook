// Package outbox delivers recorded events to the downstream app with retries.
package outbox

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"
)

type Worker struct {
	DB          *sql.DB
	URL         string
	MaxAttempts int
	Batch       int
	Poll        time.Duration
	BaseDelay   time.Duration // first retry delay; doubles each attempt
	MaxDelay    time.Duration
	Client      *http.Client
}

func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.Poll)
	defer t.Stop()
	for {
		if _, err := w.Tick(ctx); err != nil && ctx.Err() == nil {
			slog.Error("outbox tick", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick delivers one batch of due rows and returns how many it handled.
// shortcut: HTTP calls happen while row locks are held; fine at low volume, claim-then-release if downstream is slow.
func (w *Worker) Tick(ctx context.Context) (int, error) {
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx,
		`SELECT id, idempotency_key, payload::text, attempts FROM outbox
		 WHERE status='pending' AND next_attempt_at <= now()
		 ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, w.Batch)
	if err != nil {
		return 0, err
	}
	type row struct {
		id       int64
		key, pay string
		attempts int
	}
	var due []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.key, &r.pay, &r.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, r := range due {
		derr := w.deliver(ctx, r.key, r.pay)
		attempts := r.attempts + 1
		switch {
		case derr == nil:
			_, err = tx.ExecContext(ctx, `UPDATE outbox SET status='delivered', attempts=$2, last_error=NULL WHERE id=$1`, r.id, attempts)
		case attempts >= w.MaxAttempts:
			slog.Error("outbox dead", "id", r.id, "err", derr)
			_, err = tx.ExecContext(ctx, `UPDATE outbox SET status='dead', attempts=$2, last_error=$3 WHERE id=$1`, r.id, attempts, derr.Error())
		default:
			_, err = tx.ExecContext(ctx,
				`UPDATE outbox SET attempts=$2, last_error=$3, next_attempt_at=now()+$4::interval WHERE id=$1`,
				r.id, attempts, derr.Error(), w.backoff(attempts).String())
		}
		if err != nil {
			return 0, err
		}
	}
	return len(due), tx.Commit()
}

// backoff is BaseDelay*2^(attempt-1), capped, with +/-25% jitter.
func (w *Worker) backoff(attempt int) time.Duration {
	d := w.BaseDelay << (attempt - 1)
	if d <= 0 || d > w.MaxDelay {
		d = w.MaxDelay
	}
	return time.Duration(float64(d) * (0.75 + rand.Float64()*0.5))
}

func (w *Worker) deliver(ctx context.Context, key, payload string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader([]byte(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	c := w.Client
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("downstream status %d", resp.StatusCode)
	}
	return nil
}
