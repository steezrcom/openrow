package flexi

import (
	"context"

	"github.com/openrow/openrow/internal/connectors/discovery"
)

// Fetcher adapts *Client to the discovery.Fetcher interface.
type Fetcher struct{ c *Client }

func NewFetcher(creds map[string]string, allowInternal bool) (*Fetcher, error) {
	c, err := NewClient(creds, allowInternal)
	if err != nil {
		return nil, err
	}
	return &Fetcher{c: c}, nil
}

func (f *Fetcher) Connect(ctx context.Context) (string, error) {
	// /evidence-list is the cheapest auth probe. Flexi doesn't expose
	// vendor version via JSON by default; we return empty string for now.
	if _, err := f.c.ListEvidences(ctx); err != nil {
		return "", err
	}
	return "", nil
}

func (f *Fetcher) ListEvidences(ctx context.Context) ([]string, error) {
	return f.c.ListEvidences(ctx)
}

func (f *Fetcher) GetProperties(ctx context.Context, evidence string) ([]discovery.Property, error) {
	props, err := f.c.GetProperties(ctx, evidence)
	if err != nil {
		return nil, err
	}
	out := make([]discovery.Property, len(props))
	for i, p := range props {
		out[i] = discovery.Property{Name: p.Name, Type: p.Type, Writable: p.IsWritable}
	}
	return out, nil
}

func (f *Fetcher) GetSamples(ctx context.Context, evidence string, limit int) ([]map[string]any, error) {
	return f.c.GetSamples(ctx, evidence, limit)
}

func (f *Fetcher) Count(ctx context.Context, evidence string) (int64, error) {
	return f.c.Count(ctx, evidence)
}

// flexiHeuristic adapts the package-level lookups to the discovery.Heuristic
// interface.
type flexiHeuristic struct{}

func (flexiHeuristic) Evidence(name string) discovery.Role { return EvidenceRole(name) }
func (flexiHeuristic) Field(name string) discovery.Role    { return FieldRole(name) }

// NewHeuristic returns the package's heuristic for use by the discovery svc.
func NewHeuristic() discovery.Heuristic { return flexiHeuristic{} }
