// Package store records webhook events exactly once.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/payhook/payhook/internal/payment"
	"github.com/payhook/payhook/internal/provider"
	"github.com/payhook/payhook/migrations"
)

const (
	Processed          = "processed"
	Duplicate          = "duplicate"
	RejectedTransition = "rejected_transition"
	RejectedAmount     = "rejected_amount"
)

type Store struct{ DB *sql.DB }

func (s Store) Migrate(ctx context.Context) error {
	sqlText, err := migrations.FS.ReadFile("001_init.sql")
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, string(sqlText))
	return err
}

// Record stores ev once. In one transaction it dedupes, applies the state machine and enqueues the outbox row.
func (s Store) Record(ctx context.Context, prov string, ev provider.Event) (string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var eventPK int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO events (provider, event_id, outcome) VALUES ($1,$2,$3)
		 ON CONFLICT (provider, event_id) DO NOTHING RETURNING id`, prov, ev.ID, Processed).Scan(&eventPK)
	if err == sql.ErrNoRows {
		return Duplicate, nil
	}
	if err != nil {
		return "", err
	}

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO payments (provider, order_ref, amount) VALUES ($1,$2,$3)
		 ON CONFLICT DO NOTHING`, prov, ev.OrderRef, ev.Amount); err != nil {
		return "", err
	}
	var cur string
	var amount int64
	if err = tx.QueryRowContext(ctx,
		`SELECT status, amount FROM payments WHERE provider=$1 AND order_ref=$2 FOR UPDATE`,
		prov, ev.OrderRef).Scan(&cur, &amount); err != nil {
		return "", err
	}

	outcome := Processed
	switch {
	case ev.Status == payment.Paid && ev.Amount != amount:
		outcome = RejectedAmount
		slog.Warn("amount mismatch", "provider", prov, "order", ev.OrderRef, "event", ev.ID, "expected", amount, "got", ev.Amount)
	case payment.Transition(cur, ev.Status) != nil:
		outcome = RejectedTransition
		slog.Warn("illegal transition", "provider", prov, "order", ev.OrderRef, "event", ev.ID, "from", cur, "to", ev.Status)
	}

	if outcome == Processed {
		if _, err = tx.ExecContext(ctx,
			`UPDATE payments SET status=$3, updated_at=now() WHERE provider=$1 AND order_ref=$2`,
			prov, ev.OrderRef, ev.Status); err != nil {
			return "", err
		}
		payload, _ := json.Marshal(map[string]any{
			"provider": prov, "event_id": ev.ID, "order_ref": ev.OrderRef, "status": ev.Status, "amount": ev.Amount,
		})
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO outbox (event_pk, idempotency_key, payload) VALUES ($1,$2,$3)`,
			eventPK, ev.ID, payload); err != nil {
			return "", err
		}
	} else if _, err = tx.ExecContext(ctx, `UPDATE events SET outcome=$2 WHERE id=$1`, eventPK, outcome); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return outcome, nil
}
