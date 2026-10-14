package provider

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/payhook/payhook/internal/payment"
)

type Xendit struct{ CallbackToken string }

func (Xendit) Name() string { return "xendit" }

func (x Xendit) Verify(r *http.Request, _ []byte) error {
	got := r.Header.Get("X-Callback-Token")
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(x.CallbackToken)) != 1 {
		return ErrBadSignature
	}
	return nil
}

func (Xendit) Parse(body []byte) (Event, error) {
	var b struct {
		ID         string  `json:"id"`
		ExternalID string  `json:"external_id"`
		Status     string  `json:"status"`
		Amount     float64 `json:"amount"`
	}
	if err := json.Unmarshal(body, &b); err != nil {
		return Event{}, err
	}
	if b.ID == "" || b.ExternalID == "" {
		return Event{}, errors.New("xendit: missing id/external_id")
	}
	var st string
	switch b.Status {
	case "PAID", "SETTLED":
		st = payment.Paid
	case "PENDING":
		st = payment.Pending
	case "EXPIRED":
		st = payment.Expired
	case "FAILED":
		st = payment.Failed
	default:
		return Event{}, ErrIgnored
	}
	// id alone is shared by every status of one invoice, so include status to avoid dropping PENDING->PAID.
	return Event{ID: b.ID + ":" + b.Status, OrderRef: b.ExternalID, Status: st, Amount: int64(b.Amount)}, nil
}
