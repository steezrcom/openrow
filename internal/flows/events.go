package flows

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/openrow/openrow/internal/entities"
)

// EntityEventRouter matches entity change events to enabled flows with
// trigger_kind="entity_event" and hands them to the dispatcher.
//
// Trigger config shape: { "entity": "payments", "events": ["insert","update"] }
// A missing events list matches insert only (the common case).
type EntityEventRouter struct {
	svc        *Service
	dispatcher Dispatcher
	log        *slog.Logger
}

func NewEntityEventRouter(svc *Service, dispatcher Dispatcher, log *slog.Logger) *EntityEventRouter {
	return &EntityEventRouter{svc: svc, dispatcher: dispatcher, log: log}
}

// Handle is designed to be wired as entities.ChangeHandler. It's called
// synchronously after every row mutation, so it must not block — the
// dispatcher's Dispatch is async.
func (r *EntityEventRouter) Handle(ctx context.Context, e entities.ChangeEvent) {
	flows, err := r.svc.ListEntityEventMatches(ctx, e.TenantID, e.EntityName, e.EventKind)
	if err != nil {
		r.log.Error("entity event router: list flows", "err", err)
		return
	}
	if len(flows) == 0 {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"kind":        "entity_event",
		"entity":      e.EntityName,
		"event":       e.EventKind,
		"row_id":      e.RowID,
	})
	if err != nil {
		r.log.Error("entity event router: marshal payload", "err", err)
		return
	}
	for i := range flows {
		f := flows[i]
		if _, err := r.dispatcher.Dispatch(ctx, &f, payload); err != nil {
			r.log.Error("entity event router: dispatch failed",
				"flow_id", f.ID, "err", err)
		}
	}
}
