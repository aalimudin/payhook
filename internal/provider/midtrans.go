package provider

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/payhook/payhook/internal/payment"
)

type Midtrans struct{ ServerKey string }

type midtransBody struct {
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
}

func (Midtrans) Name() string { return "midtrans" }

func (m Midtrans) Verify(_ *http.Request, body []byte) error {
	var b midtransBody
	if err := json.Unmarshal(body, &b); err != nil || b.SignatureKey == "" {
		return ErrBadSignature
	}
	sum := sha512.Sum512([]byte(b.OrderID + b.StatusCode + b.GrossAmount + m.ServerKey))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(strings.ToLower(b.SignatureKey))) != 1 {
		return ErrBadSignature
	}
	return nil
}

func (Midtrans) Parse(body []byte) (Event, error) {
	var b midtransBody
	if err := json.Unmarshal(body, &b); err != nil {
		return Event{}, err
	}
	if b.TransactionID == "" || b.OrderID == "" {
		return Event{}, errors.New("midtrans: missing transaction_id/order_id")
	}
	// shortcut: gross_amount is whole currency units (IDR "10000.00"); fractions are dropped. Revisit for non-IDR.
	whole, _, _ := strings.Cut(b.GrossAmount, ".")
	amt, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return Event{}, errors.New("midtrans: bad gross_amount")
	}
	var st string
	switch b.TransactionStatus {
	case "settlement":
		st = payment.Paid
	case "capture":
		st = payment.Paid
		if b.FraudStatus == "challenge" {
			st = payment.Pending
		}
	case "pending", "authorize":
		st = payment.Pending
	case "deny", "cancel", "failure":
		st = payment.Failed
	case "expire":
		st = payment.Expired
	case "refund", "partial_refund":
		st = payment.Refunded
	default:
		return Event{}, ErrIgnored
	}
	return Event{ID: b.TransactionID + ":" + b.TransactionStatus, OrderRef: b.OrderID, Status: st, Amount: amt}, nil
}
