package flows

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
)

// Dispatcher enqueues a flow execution and returns the created Run up
// front, so HTTP callers have a run id to navigate to while the worker
// pool drives the agent loop asynchronously.
type Dispatcher interface {
	Dispatch(ctx context.Context, flow *Flow, triggerPayload json.RawMessage) (*Run, error)
}

// InMemoryDispatcher runs flows on a bounded pool of goroutines. A full
// queue returns an error rather than growing unbounded under a spike.
// Crash safety: every tool call and mutation writes to Postgres before
// the worker advances, so an abrupt exit leaves a half-run visible in
// the UI rather than lost.
type InMemoryDispatcher struct {
	svc     *Service
	runner  *Runner
	queue   chan queuedRun
	log     *slog.Logger
	workers int
	wg      sync.WaitGroup
}

type queuedRun struct {
	tenantID string
	flowID   string
	runID    string
}

// NewInMemoryDispatcher constructs an uninitialised dispatcher. Call Start
// to spin up workers; Stop to drain the queue on shutdown.
func NewInMemoryDispatcher(svc *Service, runner *Runner, workers, queueSize int, log *slog.Logger) *InMemoryDispatcher {
	if workers <= 0 {
		workers = 4
	}
	if queueSize <= 0 {
		queueSize = 256
	}
	return &InMemoryDispatcher{
		svc:     svc,
		runner:  runner,
		queue:   make(chan queuedRun, queueSize),
		log:     log,
		workers: workers,
	}
}

// Start spawns the worker goroutines. Workers exit when Stop is called.
func (d *InMemoryDispatcher) Start() {
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
}

// Stop closes the queue and waits for in-flight runs to finish.
func (d *InMemoryDispatcher) Stop() {
	close(d.queue)
	d.wg.Wait()
}

// Dispatch persists a Run row, enqueues it, and returns immediately.
// The returned Run has status=queued — callers poll to observe progress.
func (d *InMemoryDispatcher) Dispatch(ctx context.Context, flow *Flow, payload json.RawMessage) (*Run, error) {
	if flow == nil {
		return nil, errors.New("nil flow")
	}
	run, err := d.svc.CreateRun(ctx, flow, payload)
	if err != nil {
		return nil, err
	}
	select {
	case d.queue <- queuedRun{tenantID: flow.TenantID, flowID: flow.ID, runID: run.ID}:
		return run, nil
	default:
		// Mark the orphaned run failed so it doesn't sit as "queued" forever.
		_ = d.svc.UpdateRunProgress(ctx, run.ID, StatusFailed, run.MessageHistory, "flow dispatcher queue is full")
		return run, errors.New("flow dispatcher queue is full")
	}
}

func (d *InMemoryDispatcher) worker() {
	defer d.wg.Done()
	for q := range d.queue {
		// Fresh background context per run — decoupled from the HTTP
		// request that enqueued it. The runner enforces its own timeout.
		ctx := context.Background()
		flow, err := d.svc.Get(ctx, q.tenantID, q.flowID)
		if err != nil {
			d.log.Error("dispatcher worker: load flow", "flow_id", q.flowID, "err", err)
			_ = d.svc.UpdateRunProgress(ctx, q.runID, StatusFailed, nil, err.Error())
			continue
		}
		run, err := d.svc.GetRun(ctx, q.tenantID, q.runID)
		if err != nil {
			d.log.Error("dispatcher worker: load run", "run_id", q.runID, "err", err)
			continue
		}
		if _, err := d.runner.Drive(ctx, flow, run); err != nil {
			d.log.Error("flow run failed",
				"flow_id", q.flowID, "run_id", q.runID, "err", err)
		}
	}
}
