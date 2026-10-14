package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func req(h map[string]string) *http.Request {
	r := httptest.NewRequest("POST", "/", nil)
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return r
}

func TestMidtrans(t *testing.T) {
	m := Midtrans{ServerKey: "SB-key"}
	sum := sha512.Sum512([]byte("o1" + "200" + "10000.00" + "SB-key"))
	sig := hex.EncodeToString(sum[:])
	body := func(sig, amt string) []byte {
		return []byte(fmt.Sprintf(`{"transaction_id":"t1","transaction_status":"settlement","order_id":"o1","status_code":"200","gross_amount":%q,"signature_key":%q}`, amt, sig))
	}
	for name, c := range map[string]struct {
		body []byte
		ok   bool
	}{
		"valid":         {body(sig, "10000.00"), true},
		"tampered body": {body(sig, "1.00"), false},
		"wrong key":     {body(hex.EncodeToString(make([]byte, 64)), "10000.00"), false},
		"missing":       {[]byte(`{"order_id":"o1"}`), false},
		"not json":      {[]byte(`x`), false},
	} {
		if err := m.Verify(nil, c.body); (err == nil) != c.ok {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	ev, err := m.Parse(body(sig, "10000.00"))
	if err != nil || ev.Status != "paid" || ev.Amount != 10000 || ev.ID != "t1:settlement" {
		t.Errorf("parse: %+v %v", ev, err)
	}
}

func TestXendit(t *testing.T) {
	x := Xendit{CallbackToken: "tok"}
	for name, c := range map[string]struct {
		h  map[string]string
		ok bool
	}{
		"valid":   {map[string]string{"X-Callback-Token": "tok"}, true},
		"wrong":   {map[string]string{"X-Callback-Token": "nope"}, false},
		"missing": {nil, false},
	} {
		if err := x.Verify(req(c.h), nil); (err == nil) != c.ok {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	ev, err := x.Parse([]byte(`{"id":"i1","external_id":"o1","status":"PAID","amount":15000}`))
	if err != nil || ev.Status != "paid" || ev.Amount != 15000 || ev.ID != "i1:PAID" {
		t.Errorf("parse: %+v %v", ev, err)
	}
}

func TestStripe(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := Stripe{Secret: "whsec_x", Now: func() time.Time { return now }}
	body := []byte(`{"id":"evt_1","type":"payment_intent.succeeded","data":{"object":{"id":"pi_1","amount":500,"metadata":{"order_id":"o1"}}}}`)
	sign := func(ts int64, secret string, b []byte) string {
		mac := hmac.New(sha256.New, []byte(secret))
		fmt.Fprintf(mac, "%d.%s", ts, b)
		return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
	}
	for name, c := range map[string]struct {
		hdr  string
		body []byte
		ok   bool
	}{
		"valid":          {sign(now.Unix(), "whsec_x", body), body, true},
		"tampered body":  {sign(now.Unix(), "whsec_x", body), append([]byte(" "), body...), false},
		"wrong key":      {sign(now.Unix(), "other", body), body, false},
		"missing header": {"", body, false},
		"expired":        {sign(now.Add(-6*time.Minute).Unix(), "whsec_x", body), body, false},
	} {
		err := s.Verify(req(map[string]string{"Stripe-Signature": c.hdr}), c.body)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	ev, err := s.Parse(body)
	if err != nil || ev.OrderRef != "o1" || ev.Amount != 500 || ev.Status != "paid" {
		t.Errorf("parse: %+v %v", ev, err)
	}
	if _, err := s.Parse([]byte(`{"id":"e","type":"customer.created"}`)); err != ErrIgnored {
		t.Errorf("want ErrIgnored, got %v", err)
	}
}
