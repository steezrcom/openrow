package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Property is the vendor-agnostic shape of one column. Connector adapters
// translate vendor types (Flexi's Property, MSSQL's information_schema rows)
// into this so the downstream pipeline stays vendor-neutral.
type Property struct {
	Name     string
	Type     string
	Writable bool
}

// Fetcher abstracts the vendor-specific I/O of stages 1-3. Implementations
// live alongside connector packages (catalog/flexi/client.go).
type Fetcher interface {
	Connect(ctx context.Context) (vendorVersion string, err error)
	ListEvidences(ctx context.Context) ([]string, error)
	GetProperties(ctx context.Context, evidence string) ([]Property, error)
	GetSamples(ctx context.Context, evidence string, limit int) ([]map[string]any, error)
	Count(ctx context.Context, evidence string) (int64, error)
}

// Heuristic provides the static evidence/field → role mapping for Stage 4.
type Heuristic interface {
	Evidence(name string) Role
	Field(name string) Role
}

// PromptBuilder lets the connector inject its own Stage-5 prompt template.
type PromptBuilder func(ClassifyInput) (string, error)

type Service struct {
	Fetcher       Fetcher
	Heuristic     Heuristic
	Classifier    Classifier
	PromptBuilder PromptBuilder
}

// Run executes stages 1-7 and returns the produced artifact plus the review
// items. The caller is responsible for persisting both. State transitions
// (discovering → proposed / error) happen in the caller too.
func (s *Service) Run(ctx context.Context, b *Binding) (*Artifact, []ReviewItem, error) {
	if s.Fetcher == nil || s.Heuristic == nil {
		return nil, nil, fmt.Errorf("discovery service: Fetcher and Heuristic required")
	}
	vendorVersion, err := s.Fetcher.Connect(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connect: %w", err)
	}
	evidences, err := s.Fetcher.ListEvidences(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list evidences: %w", err)
	}
	art := &Artifact{
		Version:       1,
		Connector:     b.ConnectorID,
		VendorVersion: vendorVersion,
		DiscoveredAt:  time.Now().UTC(),
		Evidences:     make(map[string]*Evidence, len(evidences)),
	}
	for _, name := range evidences {
		ev, err := s.classifyOne(ctx, b, name)
		if err != nil {
			return nil, nil, fmt.Errorf("classify %q: %w", name, err)
		}
		art.Evidences[name] = ev
	}
	ProposeEntities(art)
	items := BuildReviewItems(art)
	return art, items, nil
}

func (s *Service) classifyOne(ctx context.Context, b *Binding, name string) (*Evidence, error) {
	props, err := s.Fetcher.GetProperties(ctx, name)
	if err != nil {
		return nil, err
	}
	count, err := s.Fetcher.Count(ctx, name)
	if err != nil {
		return nil, err
	}
	var samples []map[string]any
	if count > 0 {
		samples, err = s.Fetcher.GetSamples(ctx, name, 10)
		if err != nil {
			return nil, err
		}
	}

	role := s.Heuristic.Evidence(name)
	ev := &Evidence{
		Role:     role,
		Source:   SourceHeuristic,
		RowCount: count,
		Mirror:   PromotableRoles[role],
		Fields:   make(map[string]*Field, len(props)),
	}
	if role.IsKnown() {
		ev.Confidence = 1.0
		ev.Review = ReviewAuto
	} else if s.Classifier != nil && b.LLMClassification {
		out, err := s.callLLM(ctx, name, props, samples, count > 0)
		if err != nil {
			return nil, err
		}
		ev.Role = out.Role
		ev.Confidence = out.Confidence
		ev.Source = SourceLLM
		ev.Reasoning = out.Reasoning
		ev.Mirror = PromotableRoles[out.Role]
		ev.Review = Tier(out.Confidence, SourceLLM, false)
		for fname, hint := range out.FieldMap {
			ev.Fields[fname] = &Field{
				Role:       hint.Role,
				Confidence: hint.Confidence,
				Source:     SourceLLM,
				Reasoning:  hint.Reasoning,
				Review:     Tier(hint.Confidence, SourceLLM, isUserCustomField(fname)),
			}
		}
	} else {
		ev.Confidence = 0
		ev.Review = ReviewNeedsReview
	}

	for _, p := range props {
		if _, ok := ev.Fields[p.Name]; ok {
			ev.Fields[p.Name].Type = p.Type
			continue
		}
		role := s.Heuristic.Field(p.Name)
		f := &Field{Role: role, Type: p.Type, Source: SourceHeuristic}
		if role.IsKnown() {
			f.Confidence = 1.0
			f.Review = ReviewAuto
		} else {
			f.Review = Tier(0, SourceHeuristic, isUserCustomField(p.Name))
		}
		ev.Fields[p.Name] = f
	}
	return ev, nil
}

func (s *Service) callLLM(ctx context.Context, evidence string, props []Property,
	samples []map[string]any, hasSamples bool) (*ClassifyOutput, error) {
	propsJSON, _ := json.Marshal(props)
	in := ClassifyInput{
		EvidenceName: evidence,
		Properties:   propsJSON,
		Samples:      samples,
		HasSamples:   hasSamples,
	}
	if s.PromptBuilder != nil {
		buildEvidencePrompt = s.PromptBuilder
	}
	return ClassifyEvidence(ctx, s.Classifier, in)
}

// isUserCustomField returns true for Flexi-style customer-defined fields. The
// pattern is userFieldN@FXNNN. Generic enough to live in the discovery
// package (other vendors have similar conventions).
func isUserCustomField(name string) bool {
	return strings.HasPrefix(name, "userField") && strings.Contains(name, "@FX")
}
