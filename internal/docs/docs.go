// Package docs renders invoice-style business documents (order sheets,
// proforma invoices, final invoices, and overdue reminder emails) as
// print-ready HTML. The HTML is self-contained — inline CSS, A4 page
// geometry, ready to be printed from a browser to PDF or attached to an
// email. No external PDF engine; we stay Cgo-free.
//
// The renderer reads an invoice row from the Agency template's invoices
// entity, joins the related client and invoice_items, and fills a Czech
// template. It is exposed to the agent as a single tool:
//
//	render_document { invoice_id }
//
// The result is the HTML string; the agent can forward it to Resend
// (connector_resend_send_email.html), save it as a row attachment, or
// present it to the user who prints it to PDF.
package docs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/ai"
	"github.com/openrow/openrow/internal/entities"
)

// Provider returns an ai.ToolProvider. Register once at startup via
// Agent.AddToolProvider. Safe to hold a reference to entities.Service —
// all reads go through its tenant-aware API.
func Provider(ents *entities.Service) ai.ToolProvider {
	return func(ctx context.Context, tenantID, pgSchema string) []ai.Tool {
		return []ai.Tool{
			renderTool(tenantID, pgSchema, ents),
			reconcileTool(tenantID, pgSchema, ents),
		}
	}
}

func renderTool(tenantID, pgSchema string, ents *entities.Service) ai.Tool {
	return ai.Tool{
		Name: "render_document",
		Description: "Render an invoice-style document (order sheet / proforma / final invoice) from a row in the invoices entity. " +
			"Returns the full A4-ready HTML; the caller can print it to PDF from a browser or attach it to an email.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"invoice_id": map[string]any{"type": "string", "description": "UUID of the invoices row."},
			},
			"required": []string{"invoice_id"},
		},
		Handler: func(ctx context.Context, input json.RawMessage) ai.ExecResult {
			var req struct {
				InvoiceID string `json:"invoice_id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ai.ExecResult{Err: err}
			}
			if strings.TrimSpace(req.InvoiceID) == "" {
				return ai.ExecResult{Err: errors.New("invoice_id is required")}
			}
			html, kind, err := Render(ctx, ents, tenantID, pgSchema, req.InvoiceID)
			if err != nil {
				return ai.ExecResult{Err: err}
			}
			return ai.ExecResult{
				Summary: fmt.Sprintf("Rendered %s document", kind),
				Result: map[string]string{
					"kind": kind,
					"html": html,
				},
			}
		},
	}
}

// Render fetches the invoice row, its client, and its items, and renders
// the HTML. kind is derived from the row's "kind" column and drives the
// document title ("Objednávkový list" / "Zálohová faktura" / "Faktura").
func Render(ctx context.Context, ents *entities.Service, tenantID, pgSchema, invoiceID string) (html, kind string, err error) {
	invEnt, err := ents.Get(ctx, tenantID, "invoices")
	if err != nil {
		return "", "", fmt.Errorf("invoices entity not found — apply the agency template first: %w", err)
	}
	invRow, err := ents.GetRow(ctx, pgSchema, invEnt, invoiceID)
	if err != nil {
		return "", "", fmt.Errorf("invoice %s not found: %w", invoiceID, err)
	}

	clientName := ""
	clientAddress := ""
	clientIco := ""
	clientDic := ""
	clientEmail := ""
	if clientID, ok := invRow["client"].(string); ok && clientID != "" {
		if cliEnt, e := ents.Get(ctx, tenantID, "clients"); e == nil {
			if cli, e := ents.GetRow(ctx, pgSchema, cliEnt, clientID); e == nil {
				clientName = str(cli["name"])
				clientAddress = str(cli["billing_address"])
				clientIco = str(cli["ico"])
				clientDic = str(cli["dic"])
				clientEmail = str(cli["email"])
			}
		}
	}

	itemsEnt, err := ents.Get(ctx, tenantID, "invoice_items")
	items := []itemView{}
	if err == nil {
		allItems, _ := ents.ListRows(ctx, pgSchema, itemsEnt, entities.ListOptions{Limit: 500, SortBy: "created_at", SortDir: "asc"})
		for _, it := range allItems {
			if str(it["invoice"]) != invoiceID {
				continue
			}
			qty := numf(it["quantity"])
			unitPrice := numf(it["unit_price"])
			vatRate := numf(it["vat_rate"])
			subtotal := numf(it["subtotal"])
			if subtotal == 0 {
				subtotal = qty * unitPrice
			}
			items = append(items, itemView{
				Description: str(it["description"]),
				Quantity:    formatNum(qty),
				Unit:        str(it["unit"]),
				UnitPrice:   formatMoney(unitPrice, str(invRow["currency"])),
				VATRate:     formatPct(vatRate),
				Subtotal:    formatMoney(subtotal, str(invRow["currency"])),
			})
		}
	}

	kind = strings.ToLower(str(invRow["kind"]))
	if kind == "" {
		kind = "invoice"
	}
	title := titleForKind(kind)

	view := invoiceView{
		Title:         title,
		Kind:          kind,
		Number:        str(invRow["number"]),
		VS:            firstNonEmpty(str(invRow["variable_symbol"]), str(invRow["number"])),
		IssueDate:     formatDate(str(invRow["issue_date"])),
		DueDate:       formatDate(str(invRow["due_date"])),
		PaymentDate:   formatDate(str(invRow["payment_date"])),
		Currency:      firstNonEmpty(str(invRow["currency"]), "CZK"),
		Subtotal:      formatMoney(numf(invRow["subtotal"]), str(invRow["currency"])),
		VATAmount:     formatMoney(numf(invRow["vat_amount"]), str(invRow["currency"])),
		Total:         formatMoney(numf(invRow["total"]), str(invRow["currency"])),
		Status:        str(invRow["status"]),
		Notes:         str(invRow["notes"]),
		ClientName:    clientName,
		ClientAddress: clientAddress,
		ClientIco:     clientIco,
		ClientDic:     clientDic,
		ClientEmail:   clientEmail,
		Items:         items,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, view); err != nil {
		return "", kind, err
	}
	return buf.String(), kind, nil
}

// reconcileTool matches unpaired incoming bank_transactions against
// open invoices by variable_symbol, with a ±1 CZK amount tolerance.
// It's deterministic — safe to schedule unattended. Returns a summary
// of matches applied, unmatched rows, and amount-mismatch rows flagged
// for review.
func reconcileTool(tenantID, pgSchema string, ents *entities.Service) ai.Tool {
	return ai.Tool{
		Name: "reconcile_bank_transactions",
		Description: "Match unpaired incoming rows in bank_transactions against open invoices by variable_symbol (VS) and amount. " +
			"Updates bank_transactions.matched_invoice and invoices.status='paid' + payment_date on a match. " +
			"Rows with VS matches but wrong amount are tagged needs_review=true. Returns a summary.",
		Mutates: true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tolerance_czk": map[string]any{"type": "number", "description": "Amount tolerance in the invoice currency. Default 1."},
				"limit":         map[string]any{"type": "integer", "description": "Max transactions to scan. Default 200, max 500."},
			},
		},
		Handler: func(ctx context.Context, input json.RawMessage) ai.ExecResult {
			var req struct {
				Tolerance float64 `json:"tolerance_czk"`
				Limit     int     `json:"limit"`
			}
			if len(input) > 0 {
				_ = json.Unmarshal(input, &req)
			}
			if req.Tolerance <= 0 {
				req.Tolerance = 1
			}
			if req.Limit <= 0 || req.Limit > 500 {
				req.Limit = 200
			}

			txEnt, err := ents.Get(ctx, tenantID, "bank_transactions")
			if err != nil {
				return ai.ExecResult{Err: fmt.Errorf("bank_transactions entity not found: %w", err)}
			}
			invEnt, err := ents.Get(ctx, tenantID, "invoices")
			if err != nil {
				return ai.ExecResult{Err: fmt.Errorf("invoices entity not found: %w", err)}
			}

			allTx, err := ents.ListRows(ctx, pgSchema, txEnt, entities.ListOptions{
				Limit: req.Limit, SortBy: "booking_date", SortDir: "desc",
			})
			if err != nil {
				return ai.ExecResult{Err: err}
			}
			openInvoices, err := ents.ListRows(ctx, pgSchema, invEnt, entities.ListOptions{
				Limit: 500, SortBy: "issue_date", SortDir: "desc",
			})
			if err != nil {
				return ai.ExecResult{Err: err}
			}

			byVS := map[string][]entities.Row{}
			for _, inv := range openInvoices {
				status := strings.ToLower(str(inv["status"]))
				if status == "paid" || status == "cancelled" || status == "draft" {
					continue
				}
				if k := strings.ToLower(str(inv["kind"])); k == "order_sheet" {
					continue
				}
				vs := str(inv["variable_symbol"])
				if vs == "" {
					vs = str(inv["number"])
				}
				if vs == "" {
					continue
				}
				byVS[vs] = append(byVS[vs], inv)
			}

			matched := 0
			flagged := 0
			skipped := 0
			now := time.Now().Format("2006-01-02")

			for _, tx := range allTx {
				if str(tx["matched_invoice"]) != "" {
					skipped++
					continue
				}
				if strings.ToLower(str(tx["direction"])) != "in" {
					continue
				}
				vs := str(tx["variable_symbol"])
				if vs == "" {
					continue
				}
				candidates := byVS[vs]
				if len(candidates) == 0 {
					continue
				}
				amount := numf(tx["amount"])
				var pick entities.Row
				for _, c := range candidates {
					if abs(numf(c["total"])-amount) <= req.Tolerance {
						pick = c
						break
					}
				}
				txID := str(tx["id"])
				if pick == nil {
					// VS match but no amount match — flag for review.
					_ = ents.UpdateRow(ctx, pgSchema, txEnt, txID, map[string]string{
						"needs_review": "true",
					})
					flagged++
					continue
				}
				invID := str(pick["id"])
				if err := ents.UpdateRow(ctx, pgSchema, txEnt, txID, map[string]string{
					"matched_invoice": invID,
				}); err != nil {
					continue
				}
				if err := ents.UpdateRow(ctx, pgSchema, invEnt, invID, map[string]string{
					"status":       "paid",
					"payment_date": firstNonEmpty(str(tx["booking_date"]), now),
				}); err != nil {
					continue
				}
				matched++
			}
			return ai.ExecResult{
				Summary: fmt.Sprintf("Reconciled %d, flagged %d, already-matched %d", matched, flagged, skipped),
				Result: map[string]int{
					"matched":         matched,
					"flagged":         flagged,
					"already_matched": skipped,
				},
			}
		},
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

type invoiceView struct {
	Title         string
	Kind          string
	Number        string
	VS            string
	IssueDate     string
	DueDate       string
	PaymentDate   string
	Currency      string
	Subtotal      string
	VATAmount     string
	Total         string
	Status        string
	Notes         string
	ClientName    string
	ClientAddress string
	ClientIco     string
	ClientDic     string
	ClientEmail   string
	Items         []itemView
}

type itemView struct {
	Description string
	Quantity    string
	Unit        string
	UnitPrice   string
	VATRate     string
	Subtotal    string
}

func titleForKind(kind string) string {
	switch kind {
	case "order_sheet", "order":
		return "Objednávkový list"
	case "proforma", "advance":
		return "Zálohová faktura"
	case "invoice", "final":
		return "Faktura"
	default:
		return "Doklad"
	}
}

// --- helpers -------------------------------------------------------------

func str(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.Format("2006-01-02")
	}
	return fmt.Sprint(v)
}

func numf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, err := strconv.ParseFloat(strings.ReplaceAll(x, ",", "."), 64)
		if err == nil {
			return f
		}
	}
	return 0
}

func formatDate(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2. 1. 2006")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("2. 1. 2006")
	}
	return s
}

// formatMoney renders Czech-style currency ("1 234,50 CZK").
func formatMoney(f float64, currency string) string {
	if currency == "" {
		currency = "CZK"
	}
	return formatThousands(f) + " " + currency
}

func formatThousands(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	whole := int64(f)
	frac := f - float64(whole)
	out := strconv.FormatInt(whole, 10)
	// Group thousands with NBSP (\u00a0) so HTML doesn't break the number.
	if len(out) > 3 {
		var b strings.Builder
		first := len(out) % 3
		if first > 0 {
			b.WriteString(out[:first])
			if len(out) > first {
				b.WriteByte(' ')
			}
		}
		for i := first; i < len(out); i += 3 {
			b.WriteString(out[i : i+3])
			if i+3 < len(out) {
				b.WriteByte(' ')
			}
		}
		out = b.String()
	}
	out += "," + fmt.Sprintf("%02d", int(frac*100+0.5))
	if neg {
		out = "-" + out
	}
	return out
}

func formatNum(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

func formatPct(f float64) string {
	if f == 0 {
		return "0 %"
	}
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10) + " %"
	}
	return strconv.FormatFloat(f, 'f', 1, 64) + " %"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

var tmpl = template.Must(template.New("doc").Parse(docTemplate))

const docTemplate = `<!doctype html>
<html lang="cs">
<head>
<meta charset="utf-8">
<title>{{.Title}} {{.Number}}</title>
<style>
  @page { size: A4; margin: 18mm; }
  body { font-family: -apple-system, system-ui, "Segoe UI", Roboto, sans-serif; color: #222; font-size: 11pt; line-height: 1.45; margin: 0; }
  .doc { max-width: 190mm; margin: 0 auto; padding: 10mm 0; }
  .head { display: flex; justify-content: space-between; align-items: flex-start; border-bottom: 2px solid #222; padding-bottom: 6mm; margin-bottom: 8mm; }
  .head h1 { font-size: 22pt; margin: 0 0 2mm 0; }
  .muted { color: #666; font-size: 10pt; }
  .meta { text-align: right; font-size: 10pt; }
  .meta div { margin-bottom: 1mm; }
  .parties { display: flex; gap: 12mm; margin-bottom: 10mm; }
  .party { flex: 1; }
  .party h3 { font-size: 10pt; text-transform: uppercase; letter-spacing: 0.04em; color: #666; margin: 0 0 2mm 0; }
  .party p { margin: 0 0 1mm 0; font-size: 11pt; }
  table.items { width: 100%; border-collapse: collapse; margin-bottom: 6mm; }
  table.items th { text-align: left; font-size: 9pt; text-transform: uppercase; letter-spacing: 0.04em; color: #666; border-bottom: 1px solid #bbb; padding: 2mm 3mm; }
  table.items td { padding: 2mm 3mm; border-bottom: 1px solid #eee; vertical-align: top; }
  table.items td.num, table.items th.num { text-align: right; white-space: nowrap; }
  .totals { margin-left: auto; width: 70mm; font-size: 11pt; }
  .totals div { display: flex; justify-content: space-between; padding: 1.5mm 0; }
  .totals .grand { border-top: 2px solid #222; padding-top: 2mm; margin-top: 2mm; font-weight: 600; font-size: 13pt; }
  .notes { margin-top: 10mm; font-size: 10pt; color: #444; white-space: pre-wrap; }
  .status { display: inline-block; padding: 1mm 3mm; border-radius: 2mm; background: #eee; font-size: 9pt; text-transform: uppercase; letter-spacing: 0.04em; color: #444; }
  .kind-order_sheet .head { border-bottom-color: #8a5cf6; }
  .kind-proforma .head { border-bottom-color: #f59e0b; }
  .kind-invoice .head { border-bottom-color: #10b981; }
</style>
</head>
<body>
<div class="doc kind-{{.Kind}}">
  <div class="head">
    <div>
      <h1>{{.Title}}</h1>
      <div class="muted">č. {{.Number}}{{if .Status}} &middot; <span class="status">{{.Status}}</span>{{end}}</div>
    </div>
    <div class="meta">
      {{if .IssueDate}}<div><strong>Vystaveno:</strong> {{.IssueDate}}</div>{{end}}
      {{if .DueDate}}<div><strong>Splatnost:</strong> {{.DueDate}}</div>{{end}}
      {{if .PaymentDate}}<div><strong>Zaplaceno:</strong> {{.PaymentDate}}</div>{{end}}
      {{if .VS}}<div><strong>VS:</strong> {{.VS}}</div>{{end}}
    </div>
  </div>

  <div class="parties">
    <div class="party">
      <h3>Odběratel</h3>
      <p><strong>{{.ClientName}}</strong></p>
      {{if .ClientAddress}}<p>{{.ClientAddress}}</p>{{end}}
      {{if .ClientIco}}<p>IČO: {{.ClientIco}}</p>{{end}}
      {{if .ClientDic}}<p>DIČ: {{.ClientDic}}</p>{{end}}
      {{if .ClientEmail}}<p class="muted">{{.ClientEmail}}</p>{{end}}
    </div>
  </div>

  {{if .Items}}
  <table class="items">
    <thead>
      <tr>
        <th>Popis</th>
        <th class="num">Množství</th>
        <th>Jednotka</th>
        <th class="num">Cena/ks</th>
        <th class="num">DPH</th>
        <th class="num">Celkem</th>
      </tr>
    </thead>
    <tbody>
      {{range .Items}}
      <tr>
        <td>{{.Description}}</td>
        <td class="num">{{.Quantity}}</td>
        <td>{{.Unit}}</td>
        <td class="num">{{.UnitPrice}}</td>
        <td class="num">{{.VATRate}}</td>
        <td class="num">{{.Subtotal}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{end}}

  <div class="totals">
    <div><span>Základ</span><span>{{.Subtotal}}</span></div>
    <div><span>DPH</span><span>{{.VATAmount}}</span></div>
    <div class="grand"><span>Celkem k úhradě</span><span>{{.Total}}</span></div>
  </div>

  {{if .Notes}}<div class="notes">{{.Notes}}</div>{{end}}
</div>
</body>
</html>`
