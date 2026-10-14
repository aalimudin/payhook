// Package provider verifies and parses webhooks from payment providers.
package provider

import (
	"errors"
	"net/http"
)

// Event is the provider-neutral view of a webhook.
type Event struct {
	ID       string // unique per delivery-worthy event, used for dedupe
	OrderRef string
	Status   string // payment.Pending, Paid, ...
	Amount   int64  // minor units as sent by the provider
}

type Provider interface {
	Name() string
	Verify(r *http.Request, body []byte) error
	Parse(body []byte) (Event, error)
}

var (
	ErrBadSignature = errors.New("invalid signature")
	ErrIgnored      = errors.New("event type ignored")
)
