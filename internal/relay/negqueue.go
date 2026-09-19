package relay

import (
	"context"
	"sync"

	"github.com/michmich112/congee/internal/nostr"
)

type negOpenJob struct {
	ctx context.Context
	c   *Conn
	msg *nostr.NegOpenMessage
}

// NegQueue runs NEG-OPEN DB loads off the WebSocket read loop.
type NegQueue struct {
	srv  *Server
	jobs chan *negOpenJob
	ctx  context.Context
	cancel context.CancelFunc
	wg   sync.WaitGroup
}

func newNegQueue(srv *Server) *NegQueue {
	ctx, cancel := context.WithCancel(srv.metricsCtx)
	return &NegQueue{
		srv:    srv,
		jobs:   make(chan *negOpenJob, 64),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (q *NegQueue) start() {
	q.wg.Add(1)
	go q.worker()
}

func (q *NegQueue) stop() {
	q.cancel()
	q.wg.Wait()
}

func (q *NegQueue) Enqueue(job *negOpenJob) bool {
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

// PendingDepth returns waiting NEG-OPEN jobs.
func (q *NegQueue) PendingDepth() int {
	return len(q.jobs)
}

func (q *NegQueue) worker() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case job, ok := <-q.jobs:
			if !ok {
				return
			}
			q.srv.runNegOpenJob(job)
		}
	}
}
