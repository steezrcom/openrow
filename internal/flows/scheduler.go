package flows

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// ParseCron validates a cron expression. We use the standard 5-field
// format (minute hour day-of-month month day-of-week), same as Linux
// crontab. Returns a parsed schedule usable to compute Next().
func ParseCron(expr string) (cron.Schedule, error) {
	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	return p.Parse(expr)
}

// Scheduler polls for due cron-triggered flows and hands them to the
// dispatcher. A single goroutine is enough — cron granularity is
// minutes, so a 30s tick is plenty precise. Postgres does the heavy
// lifting via the flows_cron_due_idx partial index.
type Scheduler struct {
	svc        *Service
	dispatcher Dispatcher
	log        *slog.Logger
	interval   time.Duration
	done       chan struct{}
}

func NewScheduler(svc *Service, dispatcher Dispatcher, log *slog.Logger) *Scheduler {
	return &Scheduler{
		svc:        svc,
		dispatcher: dispatcher,
		log:        log,
		interval:   30 * time.Second,
		done:       make(chan struct{}),
	}
}

// Start runs the scheduler loop until ctx is cancelled. Non-blocking.
func (s *Scheduler) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)
	// Fire once at startup so we catch anything that came due while the
	// server was off, then fall into the tick cadence.
	s.tick(ctx)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// Wait blocks until the scheduler's loop exits. Useful for graceful shutdown.
func (s *Scheduler) Wait() { <-s.done }

func (s *Scheduler) tick(ctx context.Context) {
	due, err := s.svc.dueCronFlows(ctx)
	if err != nil {
		s.log.Error("scheduler: list due flows", "err", err)
		return
	}
	for i := range due {
		f := due[i]
		expr := cronExpression(f.TriggerConfig)
		schedule, perr := ParseCron(expr)
		if perr != nil {
			s.log.Error("scheduler: bad cron", "flow_id", f.ID, "expr", expr, "err", perr)
			// Push next_run_at out so we don't busy-loop on bad expressions.
			if err := s.svc.setNextRun(ctx, f.ID, time.Now().Add(1*time.Hour)); err != nil {
				s.log.Error("scheduler: clear due", "err", err)
			}
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"kind":       "cron",
			"scheduled":  time.Now().UTC().Format(time.RFC3339),
			"expression": expr,
		})
		if _, err := s.dispatcher.Dispatch(ctx, &f, payload); err != nil {
			s.log.Error("scheduler: dispatch", "flow_id", f.ID, "err", err)
			// If the queue was full we'll retry on the next tick.
			continue
		}
		next := schedule.Next(time.Now())
		if err := s.svc.setNextRun(ctx, f.ID, next); err != nil {
			s.log.Error("scheduler: advance next_run_at", "flow_id", f.ID, "err", err)
		}
	}
}

// cronExpression extracts the cron string from a flow's trigger_config
// JSON. Returns empty string if absent — callers treat that as invalid.
func cronExpression(cfg json.RawMessage) string {
	if len(cfg) == 0 {
		return ""
	}
	var m struct {
		Cron string `json:"cron"`
	}
	if err := json.Unmarshal(cfg, &m); err != nil {
		return ""
	}
	return m.Cron
}

