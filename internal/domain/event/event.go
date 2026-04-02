package event

type Base interface {
	Outboxer

	Topic() string
}
