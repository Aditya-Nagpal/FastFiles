package models

type PendingEvent struct {
	ID          int64
	EventID     string
	AggregateID string
	Payload     []byte
}
