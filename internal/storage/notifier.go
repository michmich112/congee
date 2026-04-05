package storage

// EventNotifier signals subscribers when a new event is persisted (e.g. PostgreSQL LISTEN/NOTIFY).
// Listen yields event IDs from other relay instances (same-origin notifications are filtered by the implementation).
// Close releases listener resources; it is safe to call more than once.
type EventNotifier interface {
	Notify(eventID string)
	Listen() <-chan string
	Close() error
}

// NoopNotifier does nothing; appropriate for single-instance SQLite.
type NoopNotifier struct{}

func (NoopNotifier) Notify(string) {}

func (NoopNotifier) Listen() <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}

func (NoopNotifier) Close() error { return nil }
