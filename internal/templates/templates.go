package templates

import (
	"context"
	"sort"

	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/reports"
)

// Installer installs a template in the caller's tenant/schema using the
// passed services. It should be idempotent from a "partially applied" point
// of view only at the caller's discretion: by default, it'll fail on the
// first conflicting entity name and leave earlier entities created.
type Installer func(ctx context.Context, tenantID, pgSchema string, ents *entities.Service, reps *reports.Service) error

// FlowSeed is a pre-built automation shipped with a template. It's pure
// data so the templates package can stay free of a flows.Service import
// (which would create a cycle through ai → templates → flows → ai). The
// caller — httpapi when applying a template — translates these into
// flows.CreateFlowInput and installs them after the entities exist.
type FlowSeed struct {
	Name          string
	Description   string
	Goal          string
	TriggerKind   string
	TriggerConfig map[string]any
	ToolAllowlist []string
	Mode          string
	Enabled       bool
}

type Template struct {
	ID          string
	Name        string
	Description string
	Install     Installer
	FlowSeeds   []FlowSeed
}

var registry = map[string]*Template{}

func Register(t *Template) {
	registry[t.ID] = t
}

func Get(id string) (*Template, bool) {
	t, ok := registry[id]
	return t, ok
}

func All() []*Template {
	out := make([]*Template, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
