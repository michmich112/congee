package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/uptrace/bun"
)

// Notifier implements storage.EventNotifier using pg_notify and a dedicated pgx LISTEN connection.
type Notifier struct {
	db     *bun.DB
	dsn    string
	origin string

	mu       sync.Mutex
	ch       chan string
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closeOnce sync.Once
}

type notifyPayload struct {
	ID     string `json:"id"`
	Origin string `json:"origin"`
}

// NewNotifier starts a background LISTEN loop. Close shuts it down.
func NewNotifier(db *bun.DB, dsn, origin string) (*Notifier, error) {
	if db == nil {
		return nil, errors.New("postgres: notifier: nil db")
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &Notifier{
		db:     db,
		dsn:    dsn,
		origin: origin,
		ch:     make(chan string, 256),
		cancel: cancel,
	}
	n.wg.Add(1)
	go n.listenLoop(ctx)
	return n, nil
}

// Notify publishes new_event after a successful local write (other instances should fan out).
func (n *Notifier) Notify(eventID string) {
	p := notifyPayload{ID: eventID, Origin: n.origin}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	_, _ = n.db.ExecContext(context.Background(), `SELECT pg_notify('new_event', ?)`, string(b))
}

// Listen returns a channel of event IDs originating from other instances.
func (n *Notifier) Listen() <-chan string {
	return n.ch
}

func (n *Notifier) listenLoop(ctx context.Context) {
	defer n.wg.Done()
	defer close(n.ch)

	cfg, err := pgx.ParseConfig(n.dsn)
	if err != nil {
		return
	}

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		conn, err := pgx.ConnectConfig(ctx, cfg)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}
		backoff = time.Second

		if _, err := conn.Exec(ctx, `LISTEN new_event`); err != nil {
			_ = conn.Close(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		for {
			note, err := conn.WaitForNotification(ctx)
			if err != nil {
				_ = conn.Close(ctx)
				if errors.Is(err, context.Canceled) {
					return
				}
				break // reconnect
			}
			if note == nil {
				continue
			}
			var p notifyPayload
			if err := json.Unmarshal([]byte(note.Payload), &p); err != nil {
				continue
			}
			if p.Origin == n.origin || p.ID == "" {
				continue
			}
			select {
			case n.ch <- p.ID:
			default:
			}
		}
	}
}

// Close stops the listener and waits for the goroutine to exit.
func (n *Notifier) Close() error {
	n.closeOnce.Do(func() {
		n.cancel()
		n.wg.Wait()
	})
	return nil
}
