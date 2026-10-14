package provider

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/payhook/payhook/internal/payment"
)

var ErrBadTimestamp = errors.New("invalid X-TIMESTAMP")

// SNAP verifies BI-SNAP virtual-account payment notifications (POST /v1.0/transfer-va/payment).
// The sender signs with its private key; we hold its public key. One type serves any SNAP provider.
type SNAP struct {
	ProviderName string // e.g. "midtrans-snap"
	Key          *rsa.PublicKey
}

func NewSNAP(name, publicKeyPEM string) (*SNAP, error) {
	blk, _ := pem.Decode([]byte(publicKeyPEM))
	if blk == nil {
		return nil, errors.New("snap: public key is not PEM")
	}
	k, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("snap: public key is not RSA")
	}
	return &SNAP{ProviderName: name, Key: rk}, nil
}

func (s *SNAP) Name() string { return s.ProviderName }

// Verify checks SHA256withRSA over
// "METHOD:endpoint:lowercase-hex(sha256(minified body)):X-TIMESTAMP".
// shortcut: no timestamp-age check (dedupe already blocks replays); add a tolerance if the provider documents one.
func (s *SNAP) Verify(r *http.Request, body []byte) error {
	ts, sigB64 := r.Header.Get("X-TIMESTAMP"), r.Header.Get("X-SIGNATURE")
	if sigB64 == "" {
		return ErrBadSignature
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		return ErrBadTimestamp
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return ErrBadSignature
	}
	var min bytes.Buffer
	if err := json.Compact(&min, body); err != nil {
		return ErrBadSignature
	}
	bodyHash := sha256.Sum256(min.Bytes())
	toSign := r.Method + ":" + r.URL.RequestURI() + ":" + hex.EncodeToString(bodyHash[:]) + ":" + ts
	digest := sha256.Sum256([]byte(toSign))
	if rsa.VerifyPKCS1v15(s.Key, crypto.SHA256, digest[:], sig) != nil {
		return ErrBadSignature
	}
	return nil
}

type snapBody struct {
	PartnerServiceID string `json:"partnerServiceId"`
	CustomerNo       string `json:"customerNo"`
	VirtualAccountNo string `json:"virtualAccountNo"`
	TrxID            string `json:"trxId"`
	PaymentRequestID string `json:"paymentRequestId"`
	PaidAmount       struct {
		Value string `json:"value"`
	} `json:"paidAmount"`
	AdditionalInfo struct {
		PaymentFlagStatus string `json:"paymentFlagStatus"`
	} `json:"additionalInfo"`
}

func (s *SNAP) Parse(body []byte) (Event, error) {
	var b snapBody
	if err := json.Unmarshal(body, &b); err != nil {
		return Event{}, err
	}
	flag := b.AdditionalInfo.PaymentFlagStatus
	if b.TrxID == "" || flag == "" {
		return Event{}, errors.New("snap: missing trxId/paymentFlagStatus")
	}
	var st string
	switch flag {
	case "00":
		st = payment.Paid
	case "01", "02", "03":
		st = payment.Pending
	case "04":
		st = payment.Refunded
	case "05", "06", "09":
		st = payment.Failed
	case "08":
		st = payment.Expired
	default: // 07 not found, unknown codes
		return Event{}, ErrIgnored
	}
	// paidAmount is only present once paid; "12345678.00" -> whole units (shortcut: IDR only).
	var amt int64
	if b.PaidAmount.Value != "" {
		whole, _, _ := strings.Cut(b.PaidAmount.Value, ".")
		var err error
		if amt, err = strconv.ParseInt(whole, 10, 64); err != nil {
			return Event{}, errors.New("snap: bad paidAmount.value")
		}
	}
	id := b.PaymentRequestID
	if id == "" {
		id = b.TrxID
	}
	return Event{ID: id + ":" + flag, OrderRef: b.TrxID, Status: st, Amount: amt}, nil
}

// Ack builds the success body SNAP senders expect, echoing the VA identifiers.
func (s *SNAP) Ack(body []byte) map[string]any {
	var b snapBody
	json.Unmarshal(body, &b) // already parsed OK once; echo is best-effort
	return map[string]any{
		"responseCode": "2002500", "responseMessage": "Successful",
		"virtualAccountData": map[string]string{
			"partnerServiceId": b.PartnerServiceID, "customerNo": b.CustomerNo,
			"virtualAccountNo": b.VirtualAccountNo, "trxId": b.TrxID,
		},
	}
}
