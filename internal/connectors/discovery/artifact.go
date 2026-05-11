package discovery

import "time"

// Source identifies what produced a mapping entry. User edits are authoritative
// on merge; heuristic and llm entries are refreshed on re-discovery.
type Source string

const (
	SourceHeuristic Source = "heuristic"
	SourceLLM       Source = "llm"
	SourceUser      Source = "user"
)

type Review string

const (
	ReviewAuto              Review = "auto"
	ReviewAutoLowConfidence Review = "auto_low_confidence"
	ReviewNeedsReview       Review = "needs_review"
)

type Artifact struct {
	Version       int                  `json:"version"`
	Connector     string               `json:"connector"`
	VendorVersion string               `json:"vendor_version,omitempty"`
	DiscoveredAt  time.Time            `json:"discovered_at"`
	Evidences     map[string]*Evidence `json:"evidences"`
}

type Evidence struct {
	Role          Role              `json:"role"`
	Confidence    float64           `json:"confidence"`
	Source        Source            `json:"source"`
	Mirror        bool              `json:"mirror"`
	Review        Review            `json:"review"`
	RowCount      int64             `json:"row_count"`
	Reasoning     string            `json:"reasoning,omitempty"`
	Fields        map[string]*Field `json:"fields"`
	OpenRowEntity *EntityProposal   `json:"openrow_entity,omitempty"`
}

type Field struct {
	Role       Role    `json:"role"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Source     Source  `json:"source"`
	Review     Review  `json:"review,omitempty"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

type EntityProposal struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Promoted    bool   `json:"promoted"`
	Renamed     bool   `json:"renamed,omitempty"`
}

// Merge folds fresh discovery output into a user-edited prior artifact.
// SourceUser entries are never overwritten; heuristic and LLM entries are
// replaced by their fresh counterparts. Evidences/fields missing from fresh
// are dropped (they no longer exist in the source system).
func (a *Artifact) Merge(fresh *Artifact) {
	a.Version = fresh.Version
	a.Connector = fresh.Connector
	a.VendorVersion = fresh.VendorVersion
	a.DiscoveredAt = fresh.DiscoveredAt

	merged := make(map[string]*Evidence, len(fresh.Evidences))
	for name, freshEv := range fresh.Evidences {
		priorEv := a.Evidences[name]
		if priorEv == nil {
			merged[name] = freshEv
			continue
		}
		out := *freshEv
		out.Fields = mergeFields(priorEv.Fields, freshEv.Fields)
		merged[name] = &out
	}
	a.Evidences = merged
}

func mergeFields(prior, fresh map[string]*Field) map[string]*Field {
	out := make(map[string]*Field, len(fresh))
	for name, freshF := range fresh {
		if pf, ok := prior[name]; ok && pf.Source == SourceUser {
			out[name] = pf
			continue
		}
		out[name] = freshF
	}
	return out
}
