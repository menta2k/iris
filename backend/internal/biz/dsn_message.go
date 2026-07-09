package biz

import "time"

// DSNMessage is a raw asynchronous bounce (DSN) captured at the bounce domain.
// It is retained so an operator can inspect the full notification that led to a
// dsn-sourced suppression. Recipient is the resolved original recipient (the
// suppression value), not the VERP envelope address.
type DSNMessage struct {
	ID         string
	Recipient  string
	MessageID  string
	RawMessage string
	ReceivedAt time.Time
}
