package discovery

import (
	"encoding/json"
	"testing"
	"time"
)

func TestArtifactRoundTrip(t *testing.T) {
	t.Parallel()
	a := &Artifact{
		Version:       1,
		Connector:     "abra-flexi",
		DiscoveredAt:  time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
		VendorVersion: "2024.5.3",
		Evidences: map[string]*Evidence{
			"adresar": {
				Role:       RoleCustomer,
				Confidence: 1.0,
				Source:     SourceHeuristic,
				Mirror:     true,
				Review:     ReviewAuto,
				RowCount:   1284,
				Fields: map[string]*Field{
					"nazev": {Role: RoleDisplayName, Type: "string", Confidence: 1.0, Source: SourceHeuristic},
				},
				OpenRowEntity: &EntityProposal{Name: "customer", DisplayName: "Zákazník"},
			},
		},
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Artifact
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Evidences["adresar"].Role != RoleCustomer {
		t.Fatalf("role lost in roundtrip: %q", got.Evidences["adresar"].Role)
	}
	if got.Evidences["adresar"].Fields["nazev"].Role != RoleDisplayName {
		t.Fatalf("field role lost in roundtrip")
	}
}

func TestArtifactMergePreservesUserEdits(t *testing.T) {
	t.Parallel()
	prior := &Artifact{
		Version:   1,
		Connector: "abra-flexi",
		Evidences: map[string]*Evidence{
			"adresar": {
				Role:   RoleCustomer,
				Source: SourceHeuristic,
				Fields: map[string]*Field{
					"userField1@FX001": {Role: "loyalty_tier", Source: SourceUser, Confidence: 1.0},
					"userField2@FX001": {Role: RoleUnknown, Source: SourceLLM, Confidence: 0.3},
				},
			},
		},
	}
	fresh := &Artifact{
		Version:   1,
		Connector: "abra-flexi",
		Evidences: map[string]*Evidence{
			"adresar": {
				Role:   RoleCustomer,
				Source: SourceHeuristic,
				Fields: map[string]*Field{
					"userField1@FX001": {Role: RoleUnknown, Source: SourceLLM, Confidence: 0.2},
					"userField2@FX001": {Role: RoleUnknown, Source: SourceLLM, Confidence: 0.4},
					"userField3@FX001": {Role: RoleUnknown, Source: SourceLLM, Confidence: 0.5},
				},
			},
		},
	}
	prior.Merge(fresh)
	f := prior.Evidences["adresar"].Fields
	if f["userField1@FX001"].Role != "loyalty_tier" {
		t.Errorf("user edit clobbered: %#v", f["userField1@FX001"])
	}
	if f["userField2@FX001"].Confidence != 0.4 {
		t.Errorf("llm entry should refresh: got %v", f["userField2@FX001"].Confidence)
	}
	if _, ok := f["userField3@FX001"]; !ok {
		t.Errorf("new field not added on merge")
	}
}
