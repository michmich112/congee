package sqlitewriter

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

func newTestQueue(t *testing.T, dsn string, opts Options) *Queue {
	t.Helper()
	if !HasLibsqlDriver() {
		t.Skip("libsql driver not available")
	}
	sqldb, db, err := OpenLibsqlHandles(context.Background(), dsn, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if opts.Engine == "" {
		opts.Engine = "test"
	}
	if opts.Log.GetLevel() == zerolog.NoLevel {
		opts.Log = zerolog.Nop()
	}
	opts.DSN = dsn
	if opts.OpenHandles == nil {
		opts.OpenHandles = OpenLibsqlHandles
	}
	return New(sqldb, db, opts)
}

func TestRunWriteCompletes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	q := newTestQueue(t, dir+"/test.db", Options{})
	defer func() { _ = q.Close() }()

	var ran atomic.Bool
	err := q.RunWrite(ctx, "test-op", func(ctx context.Context, db bun.IDB) error {
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
	q := newTestQueue(t, dir+"/panic.db", Options{TaskTimeout: 5 * time.Second})
	defer func() { _ = q.Close() }()

	err := q.RunWrite(ctx, "panic-op", func(ctx context.Context, db bun.IDB) error {
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

func TestRunWriteHardTimeoutUnblocksWriter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	dsn := dir + "/hard-timeout.db"
	var reconnects atomic.Int32
	opener := func(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
		reconnects.Add(1)
		return OpenLibsqlHandles(ctx, dsn, log)
	}
	q := newTestQueue(t, dsn, Options{TaskTimeout: 50 * time.Millisecond, OpenHandles: opener})
	defer func() { _ = q.Close() }()

	inTx := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- q.RunWrite(ctx, "stuck-in-tx", func(ctx context.Context, db bun.IDB) error {
			return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				close(inTx)
				<-release
				return nil
			})
		})
	}()
	<-inTx

	select {
	case err := <-firstDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want DeadlineExceeded, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hard timeout did not return")
	}
	if n := reconnects.Load(); n != 0 {
		t.Fatalf("reconnect must not run while task is in flight, got %d", n)
	}

	var secondStarted atomic.Bool
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- q.RunWrite(ctx, "after-hard-timeout", func(ctx context.Context, db bun.IDB) error {
			secondStarted.Store(true)
			return nil
		})
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("follow-up write must not start until stuck task is released: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if secondStarted.Load() {
		t.Fatal("follow-up write ran while stuck task still in flight")
	}
	if n := reconnects.Load(); n != 0 {
		t.Fatalf("reconnect must not run while task is in flight, got %d", n)
	}

	close(release)

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("writer not healthy after hard timeout: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("follow-up write did not run after stuck task released")
	}
}

func TestRunWriteTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	q := newTestQueue(t, dir+"/timeout.db", Options{TaskTimeout: 50 * time.Millisecond})
	defer func() { _ = q.Close() }()

	start := time.Now()
	err := q.RunWrite(ctx, "slow-op", func(ctx context.Context, db bun.IDB) error {
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
	block := make(chan struct{})
	writerBusy := make(chan struct{})
	q := newTestQueue(t, dir+"/full.db", Options{
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
