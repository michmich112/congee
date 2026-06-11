package sqlitewriter

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

func TestRunWriteCompletes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	sqldb, db, err := OpenHandles(ctx, dir+"/test.db", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	q := New(sqldb, db, Options{Engine: "test", Log: zerolog.Nop(), DSN: dir + "/test.db"})
	defer func() { _ = q.Close() }()

	var ran atomic.Bool
	err = q.RunWrite(ctx, "test-op", func(ctx context.Context, db bun.IDB) error {
		ran.Store(true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ran.Load() {
		t.Fatal("task did not run")
	}
}

func TestRunWritePanicRecovered(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	sqldb, db, err := OpenHandles(ctx, dir+"/panic.db", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	q := New(sqldb, db, Options{
		Engine:      "test",
		Log:         zerolog.Nop(),
		DSN:         dir + "/panic.db",
		TaskTimeout: 5 * time.Second,
	})
	defer func() { _ = q.Close() }()

	err = q.RunWrite(ctx, "panic-op", func(ctx context.Context, db bun.IDB) error {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected error after panic")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("unexpected err: %v", err)
	}

	// Writer should still accept work after panic.
	err = q.RunWrite(ctx, "after-panic", func(ctx context.Context, db bun.IDB) error {
		return nil
	})
	if err != nil {
		t.Fatalf("writer not healthy after panic: %v", err)
	}
}

func TestRunWriteTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	sqldb, db, err := OpenHandles(ctx, dir+"/timeout.db", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	q := New(sqldb, db, Options{
		Engine:      "test",
		Log:         zerolog.Nop(),
		DSN:         dir + "/timeout.db",
		TaskTimeout: 50 * time.Millisecond,
	})
	defer func() { _ = q.Close() }()

	start := time.Now()
	err = q.RunWrite(ctx, "slow-op", func(ctx context.Context, db bun.IDB) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			return nil
		}
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("timeout took too long: %v", d)
	}
}

func TestEnqueueBlocksUntilSlotFrees(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	sqldb, db, err := OpenHandles(ctx, dir+"/full.db", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	block := make(chan struct{})
	writerBusy := make(chan struct{})
	q := New(sqldb, db, Options{
		Engine:        "test",
		Log:           zerolog.Nop(),
		DSN:           dir + "/full.db",
		QueueCapacity: 1,
		TaskTimeout:   time.Second,
	})
	defer func() { _ = q.Close() }()

	// Occupy the single writer goroutine.
	go func() {
		_ = q.RunWrite(ctx, "blocker", func(ctx context.Context, db bun.IDB) error {
			close(writerBusy)
			<-block
			return nil
		})
	}()
	<-writerBusy

	// Fill the one buffered queue slot; caller blocks until the task runs.
	queuedDone := make(chan error, 1)
	go func() {
		queuedDone <- q.RunWrite(ctx, "queued", func(ctx context.Context, db bun.IDB) error {
			return nil
		})
	}()

	// Third enqueue must wait for a slot (blocker still running, channel full).
	thirdDone := make(chan error, 1)
	go func() {
		thirdDone <- q.RunWrite(ctx, "third", func(ctx context.Context, db bun.IDB) error {
			return nil
		})
	}()

	select {
	case err := <-thirdDone:
		t.Fatalf("third task should not complete while queue is full: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(block)

	select {
	case err := <-queuedDone:
		if err != nil {
			t.Fatalf("queued task failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued task did not run after blocker released")
	}

	select {
	case err := <-thirdDone:
		if err != nil {
			t.Fatalf("third task failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("third task did not run after queue slot freed")
	}
}

func TestIsReconnectable(t *testing.T) {
	t.Parallel()
	if !isReconnectable(errors.New("unable to open database file")) {
		t.Fatal("expected reconnectable")
	}
	if isReconnectable(context.DeadlineExceeded) {
		t.Fatal("timeout should not be reconnectable")
	}
	if isReconnectable(errors.New("UNIQUE constraint failed")) {
		t.Fatal("constraint errors should not trigger reconnect")
	}
}
