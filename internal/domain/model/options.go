package model

type OutboxCleanupOptions struct {
	RetentionDays  int
	BatchSize      int
	ConsumerGroups []string
	Topic          string
}
