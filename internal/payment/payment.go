// Package payment holds the payment state machine.
package payment

import "fmt"

const (
	Pending  = "pending"
	Paid     = "paid"
	Failed   = "failed"
	Expired  = "expired"
	Refunded = "refunded"
)

var allowed = map[string][]string{
	Pending: {Paid, Failed, Expired},
	Paid:    {Refunded},
}

// Transition reports whether from -> to is a legal move. Same-state is a no-op, not an error.
func Transition(from, to string) error {
	if from == to {
		return nil
	}
	for _, s := range allowed[from] {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("illegal transition %s -> %s", from, to)
}
