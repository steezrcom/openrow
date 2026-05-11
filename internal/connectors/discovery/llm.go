package discovery

import (
	"context"
	"encoding/json"
	"fmt"
)

// Classifier is the narrow LLM seam discovery uses. Implementations adapt the
// existing internal/llm client (production) or return canned JSON (tests).
type Classifier interface {
	Classify(ctx context.Context, prompt string) (string, error)
}

type ClassifyInput struct {
	EvidenceName string
	Properties   json.RawMessage
	Samples      []map[string]any
	HasSamples   bool
}

type ClassifyOutput struct {
	Role       Role                 `json:"role"`
	Confidence float64              `json:"confidence"`
	Reasoning  string               `json:"reasoning,omitempty"`
	FieldMap   map[string]FieldHint `json:"field_map,omitempty"`
}

type FieldHint struct {
	Role       Role    `json:"role"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// ClassifyEvidence calls the classifier and parses its JSON reply. Confidence
// is capped at 0.7 when HasSamples is false (Stage 5 confidence cap). A nil
// builder falls back to defaultEvidencePrompt.
func ClassifyEvidence(ctx context.Context, c Classifier, in ClassifyInput, builder PromptBuilder) (*ClassifyOutput, error) {
	if builder == nil {
		builder = defaultEvidencePrompt
	}
	prompt, err := builder(in)
	if err != nil {
		return nil, err
	}
	raw, err := c.Classify(ctx, prompt)
	if err != nil {
		return nil, err
	}
	raw = extractJSONBlock(raw)
	var out ClassifyOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode classifier reply: %w (raw=%s)", err, truncateString(raw, 200))
	}
	if !in.HasSamples && out.Confidence > 0.7 {
		out.Confidence = 0.7
	}
	for name, hint := range out.FieldMap {
		if !in.HasSamples && hint.Confidence > 0.7 {
			hint.Confidence = 0.7
			out.FieldMap[name] = hint
		}
	}
	return &out, nil
}

func defaultEvidencePrompt(in ClassifyInput) (string, error) {
	samplesB, _ := json.Marshal(in.Samples)
	return fmt.Sprintf(`You classify ERP database tables and columns into canonical semantic roles.

Evidence name: %s
Properties (raw): %s
Sample rows (up to 5, may be empty): %s

Return JSON only, matching:
{"role": "<one of: customer|supplier|product|project|cost_center|accounting_period|bank_account|invoice_outgoing|invoice_incoming|order_outgoing|order_incoming|stock_movement|bank_movement|unknown>",
 "confidence": <0.0-1.0>,
 "reasoning": "<one sentence>",
 "field_map": {"<column_name>": {"role": "<same enum + display_name|registration_id_cz|vat_id_cz|email|phone|tags|unknown>", "confidence": <0.0-1.0>}}}`,
		in.EvidenceName, string(in.Properties), string(samplesB)), nil
}

// extractJSONBlock pulls the first balanced {...} block out of a reply that
// may have prose or code fences around it.
func extractJSONBlock(s string) string {
	start := -1
	depth := 0
	for i, c := range s {
		if c == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 && start >= 0 {
				return s[start : i+1]
			}
		}
	}
	return s
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
