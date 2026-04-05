package storage

// EventNotifier signals subscribers when a new event is persisted (e.g. PostgreSQL LISTEN/NOTIFY).
type EventNotifier interface {
	Notify(eventID string)
	Listen() <-chan string
}

// NoopNotifier does nothing; appropriate for single-instance SQLite.
type NoopNotifier struct{}

func (NoopNotifier) Notify(string) {}

func (NoopNotifier) Listen() <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
