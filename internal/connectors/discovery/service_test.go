package discovery

import (
	"context"
	"testing"
)

type fakeFetcher struct {
	evidences []string
	props     map[string][]Property
	samples   map[string][]map[string]any
	counts    map[string]int64
}

func (f *fakeFetcher) Connect(ctx context.Context) (string, error)         { return "2024.5.3", nil }
func (f *fakeFetcher) ListEvidences(ctx context.Context) ([]string, error) { return f.evidences, nil }
func (f *fakeFetcher) GetProperties(ctx context.Context, e string) ([]Property, error) {
	return f.props[e], nil
}
func (f *fakeFetcher) GetSamples(ctx context.Context, e string, n int) ([]map[string]any, error) {
	return f.samples[e], nil
}
func (f *fakeFetcher) Count(ctx context.Context, e string) (int64, error) { return f.counts[e], nil }

type fakeHeuristic struct{}

func (fakeHeuristic) Evidence(name string) Role {
	if name == "adresar" {
		return RoleCustomer
	}
	return RoleUnknown
}
func (fakeHeuristic) Field(name string) Role {
	switch name {
	case "nazev":
		return RoleDisplayName
	case "ic":
		return RoleRegistrationIDCZ
	}
	return RoleUnknown
}

type fakeClassifier struct{ resp string }

func (f fakeClassifier) Classify(ctx context.Context, p string) (string, error) { return f.resp, nil }

func TestServiceRunHeuristicOnly(t *testing.T) {
	ftc := &fakeFetcher{
		evidences: []string{"adresar"},
		props: map[string][]Property{
			"adresar": {{Name: "nazev", Type: "string"}, {Name: "ic", Type: "string"}},
		},
		samples: map[string][]map[string]any{"adresar": {{"nazev": "Acme"}}},
		counts:  map[string]int64{"adresar": 1},
	}
	svc := &Service{Fetcher: ftc, Heuristic: fakeHeuristic{}, Classifier: nil}
	art, items, err := svc.Run(context.Background(), &Binding{ConnectorID: "abra-flexi"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if art.Evidences["adresar"].Role != RoleCustomer {
		t.Fatalf("expected adresar=customer, got %+v", art.Evidences["adresar"])
	}
	if len(items) != 0 {
		t.Fatalf("expected no review items, got %+v", items)
	}
	if art.VendorVersion != "2024.5.3" {
		t.Errorf("vendor_version not populated")
	}
}

func TestServiceRunLLMFillsUnknownEvidence(t *testing.T) {
	ftc := &fakeFetcher{
		evidences: []string{"verniProgram"},
		props:     map[string][]Property{"verniProgram": {{Name: "tier", Type: "string"}}},
		samples:   map[string][]map[string]any{"verniProgram": {{"tier": "Gold"}}},
		counts:    map[string]int64{"verniProgram": 1},
	}
	svc := &Service{
		Fetcher:    ftc,
		Heuristic:  fakeHeuristic{},
		Classifier: fakeClassifier{resp: `{"role":"unknown","confidence":0.4,"field_map":{}}`},
	}
	art, _, err := svc.Run(context.Background(), &Binding{ConnectorID: "abra-flexi", LLMClassification: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	ev := art.Evidences["verniProgram"]
	if ev.Source != SourceLLM || ev.Confidence != 0.4 {
		t.Fatalf("unexpected: %+v", ev)
	}
	if ev.Review != ReviewNeedsReview {
		t.Fatalf("expected needs_review tier")
	}
}
