package provider

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"strings"
	"testing"
)

const snapPath = "/webhooks/midtrans-snap/v1.0/transfer-va/payment"
const snapBodyJSON = `{"partnerServiceId":"  088899","customerNo":"123","virtualAccountNo":"  088899123","trxId":"order-1","paymentRequestId":"pr-1","paidAmount":{"value":"12345678.00","currency":"IDR"},"additionalInfo":{"paymentFlagStatus":"00"}}`

func signSNAP(t *testing.T, key *rsa.PrivateKey, path, ts string, body string) string {
	var min bytes.Buffer
	json.Compact(&min, []byte(body))
	h := sha256.Sum256(min.Bytes())
	d := sha256.Sum256([]byte("POST:" + path + ":" + hex.EncodeToString(h[:]) + ":" + ts))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, d[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

func pubPEM(t *testing.T, k *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func TestSNAPVerify(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	s, err := NewSNAP("midtrans-snap", pubPEM(t, &key.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	ts := "2026-10-14T10:00:00+07:00"
	do := func(sig, ts, body string) error {
		r := httptest.NewRequest("POST", snapPath, strings.NewReader(body))
		r.Header.Set("X-TIMESTAMP", ts)
		r.Header.Set("X-SIGNATURE", sig)
		return s.Verify(r, []byte(body))
	}
	good := signSNAP(t, key, snapPath, ts, snapBodyJSON)
	spaced := strings.ReplaceAll(snapBodyJSON, `","`, "\",\n  \"") // same JSON, different whitespace
	for name, c := range map[string]struct {
		sig, ts, body string
		want          error
	}{
		"valid":              {good, ts, snapBodyJSON, nil},
		"valid, reformatted": {good, ts, spaced, nil},
		"tampered body":      {good, ts, strings.Replace(snapBodyJSON, "12345678", "1", 1), ErrBadSignature},
		"wrong key":          {signSNAP(t, other, snapPath, ts, snapBodyJSON), ts, snapBodyJSON, ErrBadSignature},
		"wrong path":         {signSNAP(t, key, "/other", ts, snapBodyJSON), ts, snapBodyJSON, ErrBadSignature},
		"missing signature":  {"", ts, snapBodyJSON, ErrBadSignature},
		"bad timestamp":      {good, "yesterday", snapBodyJSON, ErrBadTimestamp},
	} {
		if err := do(c.sig, c.ts, c.body); err != c.want {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
}

func TestSNAPParse(t *testing.T) {
	s := &SNAP{}
	ev, err := s.Parse([]byte(snapBodyJSON))
	if err != nil || ev.Status != "paid" || ev.Amount != 12345678 || ev.OrderRef != "order-1" || ev.ID != "pr-1:00" {
		t.Errorf("paid: %+v %v", ev, err)
	}
	ev, err = s.Parse([]byte(`{"trxId":"o","additionalInfo":{"paymentFlagStatus":"08"}}`))
	if err != nil || ev.Status != "expired" || ev.Amount != 0 || ev.ID != "o:08" {
		t.Errorf("expired: %+v %v", ev, err)
	}
	if _, err := s.Parse([]byte(`{"trxId":"o","additionalInfo":{"paymentFlagStatus":"07"}}`)); err != ErrIgnored {
		t.Errorf("07: %v", err)
	}
}
