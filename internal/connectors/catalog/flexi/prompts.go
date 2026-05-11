package flexi

import (
	"encoding/json"
	"fmt"

	"github.com/openrow/openrow/internal/connectors/discovery"
)

// BuildPrompt is the Flexi-aware evidence classification prompt. Wired into
// discovery.Service when discovery is constructed for Flexi.
func BuildPrompt(in discovery.ClassifyInput) (string, error) {
	samplesB, _ := json.Marshal(in.Samples)
	return fmt.Sprintf(`You classify ABRA Flexi ERP evidences and columns into canonical semantic roles.

Flexi uses Czech identifiers (e.g. "adresar", "nazev", "ic", "dic"). Custom fields are named "userFieldN@FXNNN".

Evidence: %s
Properties JSON: %s
Sample rows (up to 5, may be empty): %s

Return JSON only:
{"role": "<one of: customer|supplier|product|project|cost_center|accounting_period|bank_account|invoice_outgoing|invoice_incoming|order_outgoing|order_incoming|stock_movement|bank_movement|unknown>",
 "confidence": <0.0-1.0>,
 "reasoning": "<one short sentence>",
 "field_map": {"<column_name>": {"role": "<role enum + display_name|registration_id_cz|vat_id_cz|email|phone|tags|unknown>", "confidence": <0.0-1.0>}}}`,
		in.EvidenceName, string(in.Properties), string(samplesB)), nil
}
