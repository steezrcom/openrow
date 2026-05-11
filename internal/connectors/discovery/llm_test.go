package discovery

import (
	"context"
	"encoding/json"
	"testing"
)

type stubClassifier struct {
	resp string
}

func (s *stubClassifier) Classify(ctx context.Context, prompt string) (string, error) {
	return s.resp, nil
}

func TestClassifyEvidence(t *testing.T) {
	t.Parallel()
	stub := &stubClassifier{resp: `{
		"role": "loyalty_tier",
		"confidence": 0.62,
		"reasoning": "tier-shaped strings",
		"field_map": {
			"userField1@FX001": {"role": "loyalty_tier", "confidence": 0.8},
			"userField2@FX001": {"role": "unknown", "confidence": 0.3}
		}
	}`}

	out, err := ClassifyEvidence(context.Background(), stub, ClassifyInput{
		EvidenceName: "verniProgram",
		Properties:   json.RawMessage(`[]`),
		Samples:      []map[string]any{{"k": "v"}},
		HasSamples:   true,
	})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if out.Role != "loyalty_tier" || out.Confidence != 0.62 {
		t.Fatalf("got %+v", out)
	}
	if out.FieldMap["userField1@FX001"].Role != "loyalty_tier" {
		t.Fatalf("field map missing")
	}
}

func TestClassifyConfidenceCapWithoutSamples(t *testing.T) {
	t.Parallel()
	stub := &stubClassifier{resp: `{"role":"customer","confidence":0.95}`}
	out, err := ClassifyEvidence(context.Background(), stub, ClassifyInput{
		EvidenceName: "x",
		Properties:   json.RawMessage(`[]`),
		HasSamples:   false,
	})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if out.Confidence > 0.7 {
		t.Fatalf("confidence not capped without samples: %v", out.Confidence)
	}
}

func TestWorkspaceClassifierUsesResolvedConfig(t *testing.T) {
	t.Parallel()
	t.Skip("integration: requires live LLM, run with OPENROW_LIVE_LLM_TESTS=1")
}
