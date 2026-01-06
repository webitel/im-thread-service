package model

type OutboxCleanupOptions struct {
	RetentionDays int
	BatchSize     int
	ConsumerGroup string
	Topic         string
}
