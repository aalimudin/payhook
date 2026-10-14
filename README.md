# payhook
A small, production-minded **payment webhook receiver in Go**. It accepts webhooks from Midtrans, Xendit and Stripe, verifies them, records each event exactly once, and reliably forwards it to your application.
