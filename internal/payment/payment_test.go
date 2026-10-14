package payment

import "testing"

func TestTransition(t *testing.T) {
	cases := []struct {
		from, to string
		ok       bool
	}{
		{Pending, Paid, true}, {Pending, Failed, true}, {Pending, Expired, true},
		{Paid, Refunded, true}, {Paid, Paid, true},
		{Paid, Pending, false}, // late out-of-order pending
		{Refunded, Paid, false}, {Failed, Paid, false}, {Pending, Refunded, false},
	}
	for _, c := range cases {
		if err := Transition(c.from, c.to); (err == nil) != c.ok {
			t.Errorf("%s->%s: err=%v, want ok=%v", c.from, c.to, err, c.ok)
		}
	}
}
