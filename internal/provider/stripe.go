package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/payhook/payhook/internal/payment"
)

const stripeTolerance = 5 * time.Minute

type Stripe struct {
	Secret string
	Now    func() time.Time // nil = time.Now
}

func (Stripe) Name() string { return "stripe" }

func (s Stripe) Verify(r *http.Request, body []byte) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(r.Header.Get("Stripe-Signature"), ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return ErrBadSignature
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if d := now().Sub(time.Unix(sec, 0)); d > stripeTolerance || d < -stripeTolerance {
		return errors.New("stripe: timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(s.Secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, sig := range sigs {
		if hmac.Equal([]byte(sig), []byte(want)) {
			return nil
		}
	}
	return ErrBadSignature
}

func (Stripe) Parse(body []byte) (Event, error) {
	var b struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID            string            `json:"id"`
				Amount        int64             `json:"amount"`
				PaymentIntent string            `json:"payment_intent"`
				Metadata      map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &b); err != nil {
		return Event{}, err
	}
	var st string
	switch b.Type {
	case "payment_intent.succeeded":
		st = payment.Paid
	case "payment_intent.payment_failed", "payment_intent.canceled":
		st = payment.Failed
	case "charge.refunded":
		st = payment.Refunded
	default:
		return Event{}, ErrIgnored
	}
	o := b.Data.Object
	ref := o.Metadata["order_id"]
	if ref == "" {
		ref = o.PaymentIntent
	}
	if ref == "" {
		ref = o.ID
	}
	if b.ID == "" || ref == "" {
		return Event{}, errors.New("stripe: missing event/object id")
	}
	return Event{ID: b.ID, OrderRef: ref, Status: st, Amount: o.Amount}, nil
}
