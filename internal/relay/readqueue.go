package relay

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	readerQueueCapacity = 1024
	readerPageTimeout   = 15 * time.Second
)

type reqPageJob struct {
	connID        string
	subID         string
	state         *reqQueryState
	searchEnabled bool
	pageSize      int
}

// ReaderQueue serializes paginated REQ reads so long snapshots do not block the read loop.
type ReaderQueue struct {
	srv    *Server
	jobs   chan *reqPageJob
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newReaderQueue(srv *Server) *ReaderQueue {
	ctx, cancel := context.WithCancel(srv.metricsCtx)
	return &ReaderQueue{
		srv:    srv,
		jobs:   make(chan *reqPageJob, readerQueueCapacity),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (q *ReaderQueue) start() {
	q.wg.Add(1)
	go q.worker()
}

func (q *ReaderQueue) stop() {
	q.cancel()
	q.wg.Wait()
}

// Enqueue schedules remaining REQ pages. Returns false when the queue is full (caller drains sync).
func (q *ReaderQueue) Enqueue(job *reqPageJob) bool {
	if job == nil {
		return true
	}
	select {
	case q.jobs <- job:
		return true
	default:
		return false
	}
}

func (q *ReaderQueue) worker() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case job, ok := <-q.jobs:
			if !ok {
				return
			}
			q.runJob(job)
		}
	}
}

func (q *ReaderQueue) runJob(job *reqPageJob) {
	if job == nil {
		return
	}
	s := q.srv
	v, ok := s.conns.Load(job.connID)
	if !ok {
		return
	}
	c := v.(*Conn)
	if !s.subs.IsOpen(job.connID, job.subID) {
		return
	}

	qctx, cancel := context.WithTimeout(q.ctx, readerPageTimeout)
	t0 := time.Now()
	events, hasMore, err := fetchREQPage(qctx, s.store, job.state, job.pageSize)
	cancel()
	if s.metrics != nil {
		s.metrics.RecordQueryLatency(time.Since(t0))
	}
	if err != nil {
		log := relayLogger(c, q.ctx)
		LogStoreErr(log, zerolog.ErrorLevel, "REQ.QueryPage", err, "req page query failed", func(e *zerolog.Event) {
			e.Str("sub_id", job.subID)
		})
		return
	}

	for _, ev := range events {
		if !s.EventVisibleToSubscription(job.connID, ev) {
			continue
		}
		sendErr := c.sendEvent(job.subID, ev)
		if sendErr != nil {
			if errors.Is(sendErr, ErrSlowConsumer) {
				log := relayLogger(c, q.ctx)
				log.Warn().Err(sendErr).Str("sub_id", job.subID).Str("event_id", ev.ID).Msg("send buffer full: initial event skipped")
			}
		}
		s.subs.NoteSubInitialDelivery(job.connID, job.subID, sendErr == nil)
	}

	if hasMore {
		if !q.Enqueue(job) {
			drainRemainingPages(q.ctx, s, c, job.subID, job.state, job.pageSize)
		}
		return
	}

	if err := c.sendEOSE(job.subID); err != nil {
		return
	}
	s.subs.NoteSubEOSE(job.connID, job.subID)
}

// drainRemainingPages fetches and sends all remaining REQ pages synchronously.
func drainRemainingPages(ctx context.Context, s *Server, c *Conn, subID string, state *reqQueryState, pageSize int) {
	for {
		qctx, cancel := context.WithTimeout(ctx, readerPageTimeout)
		t0 := time.Now()
		events, hasMore, err := fetchREQPage(qctx, s.store, state, pageSize)
		cancel()
		if s.metrics != nil {
			s.metrics.RecordQueryLatency(time.Since(t0))
		}
		if err != nil {
			log := relayLogger(c, ctx)
			LogStoreErr(log, zerolog.ErrorLevel, "REQ.QueryPage", err, "req page query failed", func(e *zerolog.Event) {
				e.Str("sub_id", subID)
			})
			return
		}
		for _, ev := range events {
			if !s.EventVisibleToSubscription(c.ID, ev) {
				continue
			}
			sendErr := c.sendEvent(subID, ev)
			if sendErr != nil && errors.Is(sendErr, ErrSlowConsumer) {
				log := relayLogger(c, ctx)
				log.Warn().Err(sendErr).Str("sub_id", subID).Str("event_id", ev.ID).Msg("send buffer full: initial event skipped")
			}
			s.subs.NoteSubInitialDelivery(c.ID, subID, sendErr == nil)
		}
		if !hasMore {
			break
		}
	}
}
