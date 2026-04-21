package connectors

import "sort"

var registry = map[string]*Connector{}

// Register adds a Connector descriptor to the registry. Called from each
// connector's init().
func Register(c *Connector) {
	registry[c.ID] = c
}

// Get returns the descriptor for id, or nil if unregistered.
func Get(id string) *Connector {
	return registry[id]
}

// All returns the full registry, sorted by name for stable UI ordering.
func All() []*Connector {
	out := make([]*Connector, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
