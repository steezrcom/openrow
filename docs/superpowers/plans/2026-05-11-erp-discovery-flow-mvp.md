# AI-native ERP discovery (Flexi MVP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the discovery pipeline for ABRA Flexi end-to-end: a workspace user saves Flexi creds, watches discovery run, reviews the mapping, activates the binding, and the agent gets a new tool that queries Flexi live using canonical role names. Mirror worker, entity promotion, and polished UI are out of scope here (separate follow-up plan).

**Architecture:** A new `discovery` package runs a stage-based pipeline. Stages 1–3 (network I/O) and Stage 4 heuristics live next to the Flexi connector in `internal/connectors/catalog/flexi`. Stage 5 calls the workspace-configured LLM via the existing `internal/llm` service (no per-binding model override). Bindings live in two new tables (`openrow.external_bindings`, `openrow.external_binding_review_items`); credentials reuse the existing `openrow.connector_configs` storage so Flexi appears in the existing Connectors UI list. The read-through tool is registered as an `Action` on the Flexi connector descriptor, which becomes a normal agent tool `connector.abra-flexi.query`.

**Tech Stack:** Go 1.25, `pgx/v5`, `embed` for migrations, `go-openai` for LLM, `net/http` stdlib for Flexi REST, React 18 + TanStack Router/Query for UI.

**Spec:** [docs/superpowers/specs/2026-05-11-erp-discovery-flow-design.md](../specs/2026-05-11-erp-discovery-flow-design.md)

---

## File structure

**New files:**
- `internal/store/migrations/0015_external_bindings.sql` — tables: `external_bindings`, `external_binding_review_items`, `external_binding_cursors` (cursors table created here for forward-compat with Plan 2; not used yet).
- `internal/net/ssrf.go` + `_test.go` — outbound URL guard.
- `internal/connectors/catalog/flexi/flexi.go` — connector descriptor, init() registration.
- `internal/connectors/catalog/flexi/client.go` + `_test.go` — Flexi REST HTTP client.
- `internal/connectors/catalog/flexi/heuristics.go` + `_test.go` — static evidence/field maps.
- `internal/connectors/catalog/flexi/prompts.go` — LLM prompt builders.
- `internal/connectors/catalog/flexi/action_query.go` — read-through Action implementation.
- `internal/connectors/catalog/flexi/testdata/` — recorded Flexi responses for tests.
- `internal/connectors/discovery/artifact.go` + `_test.go` — mapping artifact types.
- `internal/connectors/discovery/roles.go` — canonical role enum + promotable set.
- `internal/connectors/discovery/service.go` + `_test.go` — discovery coordinator.
- `internal/connectors/discovery/stages.go` + `_test.go` — heuristic, tier, persist, propose stages.
- `internal/connectors/discovery/llm.go` + `_test.go` — Stage 5 LLM classifier.
- `internal/connectors/discovery/repo.go` + `_test.go` — binding + review-item repository.
- `internal/httpapi/external_bindings.go` + `_test.go` — HTTP handlers.
- `web/src/routes/_app.settings.connectors.external.tsx` — list + connect form.
- `web/src/routes/_app.settings.connectors.external.$id.tsx` — discovery status + review.

**Modified files:**
- `internal/connectors/catalog/catalog.go` — add `_ ".../flexi"` blank import.
- `internal/httpapi/server.go` — mount new routes, wire dependencies.
- `cmd/server/main.go` — construct `discovery.Service` and pass to `httpapi`.

---

## Task 1: Migration — external_bindings tables

**Files:**
- Create: `internal/store/migrations/0015_external_bindings.sql`

- [ ] **Step 1: Write the migration**

```sql
-- 0015_external_bindings.sql
-- Tables backing AI-native ERP discovery (see spec at
-- docs/superpowers/specs/2026-05-11-erp-discovery-flow-design.md).

CREATE TABLE openrow.external_bindings (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    connector_id         text NOT NULL,
    state                text NOT NULL CHECK (state IN ('discovering','proposed','active','error')),
    mapping              jsonb,
    llm_classification   boolean NOT NULL DEFAULT true,
    last_error           text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, connector_id)
);

CREATE INDEX external_bindings_tenant_idx ON openrow.external_bindings (tenant_id);

CREATE TABLE openrow.external_binding_review_items (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    binding_id      uuid NOT NULL REFERENCES openrow.external_bindings(id) ON DELETE CASCADE,
    evidence        text NOT NULL,
    field           text,
    proposed        jsonb NOT NULL,
    status          text NOT NULL CHECK (status IN ('pending','accepted','edited','rejected')) DEFAULT 'pending',
    resolved_by     uuid,
    resolved_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX external_binding_review_items_binding_idx
    ON openrow.external_binding_review_items (binding_id, status);

CREATE TABLE openrow.external_binding_cursors (
    binding_id      uuid NOT NULL REFERENCES openrow.external_bindings(id) ON DELETE CASCADE,
    evidence        text NOT NULL,
    cursor_value    text NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (binding_id, evidence)
);
```

- [ ] **Step 2: Verify the migration builds and applies**

```bash
cd /Users/johnny/projects/steezrcom/openrow
go build ./...
make db-up
make seed
```

Expected: clean build, `psql` shows the three tables in the `openrow` schema. Run `psql $DATABASE_URL -c "\dt openrow.*"` to confirm.

- [ ] **Step 3: Commit**

```bash
git add internal/store/migrations/0015_external_bindings.sql
git commit -m "feat(db): migration for external bindings + review items"
```

---

## Task 2: SSRF guard

**Files:**
- Create: `internal/net/ssrf.go`
- Test: `internal/net/ssrf_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/net/ssrf_test.go
package net

import "testing"

func TestValidateOutboundURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"public https", "https://flexibee.example.com", false},
		{"http rejected by default", "http://flexibee.example.com", true},
		{"loopback", "https://127.0.0.1/", true},
		{"loopback v6", "https://[::1]/", true},
		{"link local", "https://169.254.169.254/", true},
		{"private 10.x", "https://10.0.0.5/", true},
		{"private 192.168", "https://192.168.1.10/", true},
		{"private 172.16", "https://172.16.0.1/", true},
		{"empty", "", true},
		{"bad scheme", "ftp://x.example.com/", true},
		{"missing host", "https:///path", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOutboundURL(tc.url, false)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("private allowed when allowInternal=true", func(t *testing.T) {
		if err := ValidateOutboundURL("https://10.0.0.5/", true); err != nil {
			t.Fatalf("expected nil with allowInternal=true, got %v", err)
		}
	})
}
```

- [ ] **Step 2: Run test, verify it fails**

```bash
go test ./internal/net/...
```

Expected: FAIL with "undefined: ValidateOutboundURL" or package missing.

- [ ] **Step 3: Implement ValidateOutboundURL**

```go
// internal/net/ssrf.go
package net

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateOutboundURL parses u and rejects empty, non-http(s), or URLs that
// resolve to a private/loopback/link-local address. When allowInternal is true,
// the address-range check is skipped (operator opt-in for on-prem deployments).
func ValidateOutboundURL(u string, allowInternal bool) error {
	if strings.TrimSpace(u) == "" {
		return errors.New("url is empty")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && allowInternal) {
		return fmt.Errorf("scheme %q not allowed", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("url has no host")
	}
	if allowInternal {
		return nil
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("host resolves to disallowed address %s", a)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test, verify it passes**

```bash
go test ./internal/net/...
```

Expected: PASS for all sub-tests.

- [ ] **Step 5: Commit**

```bash
git add internal/net/ssrf.go internal/net/ssrf_test.go
git commit -m "feat(net): ssrf guard for outbound urls"
```

---

## Task 3: Canonical roles + heuristics

**Files:**
- Create: `internal/connectors/discovery/roles.go`
- Create: `internal/connectors/catalog/flexi/heuristics.go`
- Test: `internal/connectors/catalog/flexi/heuristics_test.go`

- [ ] **Step 1: Write the canonical roles**

```go
// internal/connectors/discovery/roles.go
package discovery

// Role is a canonical semantic role assigned to ERP evidences/columns by the
// discovery pipeline. The enum is closed; "unknown" is the sentinel for
// LLM-classified items the model could not place.
type Role string

const (
	RoleUnknown Role = "unknown"

	RoleCustomer         Role = "customer"
	RoleSupplier         Role = "supplier"
	RoleProduct          Role = "product"
	RoleProject          Role = "project"
	RoleCostCenter       Role = "cost_center"
	RoleAccountingPeriod Role = "accounting_period"
	RoleBankAccount      Role = "bank_account"

	RoleInvoiceOutgoing Role = "invoice_outgoing"
	RoleInvoiceIncoming Role = "invoice_incoming"
	RoleOrderOutgoing   Role = "order_outgoing"
	RoleOrderIncoming   Role = "order_incoming"
	RoleStockMovement   Role = "stock_movement"
	RoleBankMovement    Role = "bank_movement"

	// Field-level roles.
	RoleDisplayName      Role = "display_name"
	RoleRegistrationIDCZ Role = "registration_id_cz" // IČO
	RoleVATIDCZ          Role = "vat_id_cz"          // DIČ
	RoleEmail            Role = "email"
	RolePhone            Role = "phone"
	RoleTags             Role = "tags"
)

// PromotableRoles is the set of entity-shaped roles that produce OpenRow
// entity proposals on activation. Transactional roles (invoices, movements)
// are accessible via read-through tool only.
var PromotableRoles = map[Role]bool{
	RoleCustomer:    true,
	RoleProduct:     true,
	RoleProject:     true,
	RoleCostCenter:  true,
	RoleBankAccount: true,
}

// IsKnown reports whether r is anything other than RoleUnknown.
func (r Role) IsKnown() bool { return r != "" && r != RoleUnknown }
```

- [ ] **Step 2: Write the failing heuristics test**

```go
// internal/connectors/catalog/flexi/heuristics_test.go
package flexi

import (
	"testing"

	"github.com/openrow/openrow/internal/connectors/discovery"
)

func TestEvidenceHeuristic(t *testing.T) {
	t.Parallel()
	cases := map[string]discovery.Role{
		"adresar":            discovery.RoleCustomer,
		"faktura-vydana":     discovery.RoleInvoiceOutgoing,
		"faktura-prijata":    discovery.RoleInvoiceIncoming,
		"cenik":              discovery.RoleProduct,
		"skladovy-pohyb":     discovery.RoleStockMovement,
		"objednavka-prijata": discovery.RoleOrderIncoming,
		"objednavka-vydana":  discovery.RoleOrderOutgoing,
		"zakazka":            discovery.RoleProject,
		"stredisko":          discovery.RoleCostCenter,
		"ucetni-obdobi":      discovery.RoleAccountingPeriod,
		"bankovni-ucet":      discovery.RoleBankAccount,
		"banka":              discovery.RoleBankMovement,
		"nonsense-evidence":  discovery.RoleUnknown,
	}
	for evidence, want := range cases {
		got := EvidenceRole(evidence)
		if got != want {
			t.Errorf("EvidenceRole(%q) = %q, want %q", evidence, got, want)
		}
	}
}

func TestFieldHeuristic(t *testing.T) {
	t.Parallel()
	cases := map[string]discovery.Role{
		"nazev":   discovery.RoleDisplayName,
		"ic":      discovery.RoleRegistrationIDCZ,
		"dic":     discovery.RoleVATIDCZ,
		"email":   discovery.RoleEmail,
		"telefon": discovery.RolePhone,
		"stitky":  discovery.RoleTags,
		"unknownColumn": discovery.RoleUnknown,
	}
	for field, want := range cases {
		got := FieldRole(field)
		if got != want {
			t.Errorf("FieldRole(%q) = %q, want %q", field, got, want)
		}
	}
}
```

- [ ] **Step 3: Run test, verify it fails**

```bash
go test ./internal/connectors/catalog/flexi/...
```

Expected: FAIL — package or functions undefined.

- [ ] **Step 4: Implement heuristics**

```go
// internal/connectors/catalog/flexi/heuristics.go
package flexi

import "github.com/openrow/openrow/internal/connectors/discovery"

// evidenceRoles is the curated static map of Flexi evidence names to canonical
// semantic roles. Add new entries via PR with the Flexi version they were
// observed on. v1 keeps adresar=customer; supplier/polymorphic mapping is out
// of scope (see spec out-of-scope list).
var evidenceRoles = map[string]discovery.Role{
	"adresar":            discovery.RoleCustomer,
	"faktura-vydana":     discovery.RoleInvoiceOutgoing,
	"faktura-prijata":    discovery.RoleInvoiceIncoming,
	"cenik":              discovery.RoleProduct,
	"skladovy-pohyb":     discovery.RoleStockMovement,
	"objednavka-prijata": discovery.RoleOrderIncoming,
	"objednavka-vydana":  discovery.RoleOrderOutgoing,
	"zakazka":            discovery.RoleProject,
	"stredisko":          discovery.RoleCostCenter,
	"ucetni-obdobi":      discovery.RoleAccountingPeriod,
	"bankovni-ucet":      discovery.RoleBankAccount,
	"banka":              discovery.RoleBankMovement,
}

var fieldRoles = map[string]discovery.Role{
	"nazev":   discovery.RoleDisplayName,
	"ic":      discovery.RoleRegistrationIDCZ,
	"dic":     discovery.RoleVATIDCZ,
	"email":   discovery.RoleEmail,
	"telefon": discovery.RolePhone,
	"stitky":  discovery.RoleTags,
}

// EvidenceRole returns the canonical role for a Flexi evidence name, or
// RoleUnknown if there is no exact match.
func EvidenceRole(evidence string) discovery.Role {
	if r, ok := evidenceRoles[evidence]; ok {
		return r
	}
	return discovery.RoleUnknown
}

// FieldRole returns the canonical role for a Flexi column name.
func FieldRole(name string) discovery.Role {
	if r, ok := fieldRoles[name]; ok {
		return r
	}
	return discovery.RoleUnknown
}
```

- [ ] **Step 5: Run test, verify it passes**

```bash
go test ./internal/connectors/catalog/flexi/... ./internal/connectors/discovery/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/discovery/roles.go internal/connectors/catalog/flexi/heuristics.go internal/connectors/catalog/flexi/heuristics_test.go
git commit -m "feat(connectors): canonical roles and flexi heuristics"
```

---

## Task 4: Mapping artifact types

**Files:**
- Create: `internal/connectors/discovery/artifact.go`
- Test: `internal/connectors/discovery/artifact_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/connectors/discovery/artifact_test.go
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
```

- [ ] **Step 2: Run test, verify it fails**

Expected: FAIL — types undefined.

- [ ] **Step 3: Implement the artifact**

```go
// internal/connectors/discovery/artifact.go
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

// Review state of an evidence or field in the mapping artifact.
type Review string

const (
	ReviewAuto              Review = "auto"
	ReviewAutoLowConfidence Review = "auto_low_confidence"
	ReviewNeedsReview       Review = "needs_review"
)

// Artifact is the per-binding mapping document persisted as JSON.
type Artifact struct {
	Version       int                  `json:"version"`
	Connector     string               `json:"connector"`
	VendorVersion string               `json:"vendor_version,omitempty"`
	DiscoveredAt  time.Time            `json:"discovered_at"`
	Evidences     map[string]*Evidence `json:"evidences"`
}

type Evidence struct {
	Role          Role             `json:"role"`
	Confidence    float64          `json:"confidence"`
	Source        Source           `json:"source"`
	Mirror        bool             `json:"mirror"`
	Review        Review           `json:"review"`
	RowCount      int64            `json:"row_count"`
	Reasoning     string           `json:"reasoning,omitempty"`
	Fields        map[string]*Field `json:"fields"`
	OpenRowEntity *EntityProposal  `json:"openrow_entity,omitempty"`
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

// Merge folds fresh discovery output into a (User-edited) prior artifact. Rule:
// SourceUser entries are never overwritten. Heuristic and LLM entries are
// replaced by their fresh counterparts. New evidences/fields appear; missing
// ones are dropped (they no longer exist in the source system).
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
```

- [ ] **Step 4: Run test, verify it passes**

```bash
go test ./internal/connectors/discovery/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/discovery/artifact.go internal/connectors/discovery/artifact_test.go
git commit -m "feat(discovery): mapping artifact types and merge"
```

---

## Task 5: Flexi connector descriptor

**Files:**
- Create: `internal/connectors/catalog/flexi/flexi.go`
- Modify: `internal/connectors/catalog/catalog.go`

- [ ] **Step 1: Add the Flexi descriptor**

```go
// internal/connectors/catalog/flexi/flexi.go
// Package flexi integrates with ABRA Flexi (formerly FlexiBee), a Czech SMB
// ERP. The REST API is self-describing via /c/<company>/evidence-list and
// per-evidence /properties endpoints — discovery uses those.
package flexi

import (
	"context"

	"github.com/openrow/openrow/internal/connectors"
)

const ConnectorID = "abra-flexi"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          ConnectorID,
		Name:        "ABRA Flexi",
		Description: "Czech SMB ERP. AI-native binding scans the evidence list and proposes a mapping the agent can query.",
		Category:    "erp",
		Homepage:    "https://www.abra.eu/flexi/",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "base_url", Label: "Server URL", Kind: connectors.FieldURL, Required: true,
				Placeholder: "https://demo.flexibee.eu",
				Help:        "Root URL of the Flexi server (no trailing /c/<company>)."},
			{Name: "company", Label: "Company token", Kind: connectors.FieldText, Required: true,
				Placeholder: "demo_s_r_o_",
				Help:        "Company segment from your Flexi URL — the part after /c/."},
			{Name: "username", Label: "Username", Kind: connectors.FieldText, Required: true},
			{Name: "password", Label: "Password", Kind: connectors.FieldSecret, Required: true},
		},
		Test:    testCreds,
		Actions: actions(),
	})
}

// testCreds validates by calling /evidence-list. Implemented in client.go.
func testCreds(ctx context.Context, creds map[string]string) error {
	c, err := NewClient(creds, false)
	if err != nil {
		return err
	}
	_, err = c.ListEvidences(ctx)
	return err
}

// actions are registered later (Task 13). Empty for now so registration works.
func actions() []connectors.Action { return nil }
```

- [ ] **Step 2: Wire into catalog**

```go
// internal/connectors/catalog/catalog.go — add the blank import line.
```

Edit `internal/connectors/catalog/catalog.go` so the import block includes:

```go
import (
	_ "github.com/openrow/openrow/internal/connectors/catalog/ares"
	_ "github.com/openrow/openrow/internal/connectors/catalog/cnb"
	_ "github.com/openrow/openrow/internal/connectors/catalog/csas"
	_ "github.com/openrow/openrow/internal/connectors/catalog/csob"
	_ "github.com/openrow/openrow/internal/connectors/catalog/discord"
	_ "github.com/openrow/openrow/internal/connectors/catalog/fakturoid"
	_ "github.com/openrow/openrow/internal/connectors/catalog/fio"
	_ "github.com/openrow/openrow/internal/connectors/catalog/flexi"
	_ "github.com/openrow/openrow/internal/connectors/catalog/github"
	_ "github.com/openrow/openrow/internal/connectors/catalog/linear"
	_ "github.com/openrow/openrow/internal/connectors/catalog/notion"
	_ "github.com/openrow/openrow/internal/connectors/catalog/resend"
	_ "github.com/openrow/openrow/internal/connectors/catalog/revolut"
	_ "github.com/openrow/openrow/internal/connectors/catalog/slack"
	_ "github.com/openrow/openrow/internal/connectors/catalog/stripe"
	_ "github.com/openrow/openrow/internal/connectors/catalog/uol"
	_ "github.com/openrow/openrow/internal/connectors/catalog/vies"
)
```

- [ ] **Step 3: Verify build (still references NewClient/ListEvidences not yet defined, so allow this step to fail and proceed)**

Note: client.go is implemented in Task 6. After Task 5 alone, `go build ./...` will fail because `NewClient` doesn't exist yet. That's expected. Skip the build check here; it runs after Task 6 finishes.

- [ ] **Step 4: Commit (without build check)**

```bash
git add internal/connectors/catalog/flexi/flexi.go internal/connectors/catalog/catalog.go
git commit -m "feat(flexi): connector descriptor"
```

---

## Task 6: Flexi HTTP client

**Files:**
- Create: `internal/connectors/catalog/flexi/client.go`
- Test: `internal/connectors/catalog/flexi/client_test.go`
- Create: `internal/connectors/catalog/flexi/testdata/evidence-list.json`
- Create: `internal/connectors/catalog/flexi/testdata/adresar-properties.json`
- Create: `internal/connectors/catalog/flexi/testdata/adresar-samples.json`

- [ ] **Step 1: Write canned response fixtures**

```json
// internal/connectors/catalog/flexi/testdata/evidence-list.json
{
  "winstrom": {
    "evidenceList": [
      {"evidenceType": "adresar", "name": "Address book"},
      {"evidenceType": "faktura-vydana", "name": "Issued invoices"},
      {"evidenceType": "cenik", "name": "Price list"}
    ]
  }
}
```

```json
// internal/connectors/catalog/flexi/testdata/adresar-properties.json
{
  "properties": {
    "property": [
      {"propertyName": "nazev", "type": "string", "isWritable": true},
      {"propertyName": "ic", "type": "string", "isWritable": true},
      {"propertyName": "dic", "type": "string", "isWritable": true},
      {"propertyName": "email", "type": "string", "isWritable": true},
      {"propertyName": "userField1@FX001", "type": "string", "isWritable": true}
    ]
  }
}
```

```json
// internal/connectors/catalog/flexi/testdata/adresar-samples.json
{
  "winstrom": {
    "adresar": [
      {"nazev": "Acme s.r.o.", "ic": "12345678", "dic": "CZ12345678", "email": "info@acme.cz", "userField1@FX001": "Gold"},
      {"nazev": "Beta a.s.", "ic": "87654321", "dic": "CZ87654321", "email": "kontakt@beta.cz", "userField1@FX001": "Silver"}
    ]
  }
}
```

- [ ] **Step 2: Write the failing client test**

```go
// internal/connectors/catalog/flexi/client_test.go
package flexi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/c/demo/evidence-list.json", func(w http.ResponseWriter, r *http.Request) {
		if user, _, _ := r.BasicAuth(); user != "user1" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		serve(w, "testdata/evidence-list.json")
	})
	mux.HandleFunc("/c/demo/adresar/properties.json", func(w http.ResponseWriter, r *http.Request) {
		serve(w, "testdata/adresar-properties.json")
	})
	mux.HandleFunc("/c/demo/adresar.json", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "limit=10") {
			http.Error(w, "missing limit", http.StatusBadRequest)
			return
		}
		serve(w, "testdata/adresar-samples.json")
	})
	mux.HandleFunc("/c/demo/adresar/$count", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"2"}}`))
	})
	return httptest.NewServer(mux)
}

func serve(w http.ResponseWriter, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func TestClientListEvidences(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	c, err := NewClient(map[string]string{
		"base_url": srv.URL,
		"company":  "demo",
		"username": "user1",
		"password": "pw",
	}, true)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	evs, err := c.ListEvidences(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(evs) != 3 || evs[0] != "adresar" {
		t.Fatalf("unexpected evidences: %+v", evs)
	}
}

func TestClientIntrospect(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	c, _ := NewClient(map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "user1", "password": "pw",
	}, true)
	props, err := c.GetProperties(context.Background(), "adresar")
	if err != nil {
		t.Fatalf("properties: %v", err)
	}
	if len(props) != 5 || props[0].Name != "nazev" {
		t.Fatalf("properties: %+v", props)
	}
	samples, err := c.GetSamples(context.Background(), "adresar", 10)
	if err != nil {
		t.Fatalf("samples: %v", err)
	}
	if len(samples) != 2 || samples[0]["nazev"] != "Acme s.r.o." {
		t.Fatalf("samples: %+v", samples)
	}
	count, err := c.Count(context.Background(), "adresar")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}
```

- [ ] **Step 3: Run test, verify it fails**

```bash
go test ./internal/connectors/catalog/flexi/...
```

Expected: FAIL — `NewClient`, `ListEvidences`, etc. not defined.

- [ ] **Step 4: Implement the client**

```go
// internal/connectors/catalog/flexi/client.go
package flexi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	openrownet "github.com/openrow/openrow/internal/net"
)

// Client talks to one Flexi server / company.
type Client struct {
	baseURL  string // root, no trailing slash
	company  string
	username string
	password string
	http     *http.Client
}

// NewClient builds a client from the credential map. allowInternal skips SSRF
// host-range checks; pass true only in tests or operator-opted-in deployments.
func NewClient(creds map[string]string, allowInternal bool) (*Client, error) {
	base := strings.TrimRight(creds["base_url"], "/")
	company := creds["company"]
	if base == "" || company == "" || creds["username"] == "" {
		return nil, errors.New("flexi: base_url, company, username are required")
	}
	if err := openrownet.ValidateOutboundURL(base, allowInternal); err != nil {
		return nil, fmt.Errorf("flexi base_url: %w", err)
	}
	return &Client{
		baseURL:  base,
		company:  company,
		username: creds["username"],
		password: creds["password"],
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := fmt.Sprintf("%s/c/%s/%s", c.baseURL, url.PathEscape(c.company), path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB cap
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("flexi %s: %s — %s", u, resp.Status, truncate(body, 200))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// ListEvidences returns the names of all evidences available on the server.
func (c *Client) ListEvidences(ctx context.Context) ([]string, error) {
	body, err := c.get(ctx, "evidence-list.json", nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Winstrom struct {
			EvidenceList []struct {
				EvidenceType string `json:"evidenceType"`
			} `json:"evidenceList"`
		} `json:"winstrom"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode evidence-list: %w", err)
	}
	out := make([]string, 0, len(env.Winstrom.EvidenceList))
	for _, e := range env.Winstrom.EvidenceList {
		out = append(out, e.EvidenceType)
	}
	return out, nil
}

// Property is one column described by Flexi's properties endpoint.
type Property struct {
	Name       string `json:"propertyName"`
	Type       string `json:"type"`
	IsWritable bool   `json:"isWritable"`
}

// GetProperties returns the schema for one evidence.
func (c *Client) GetProperties(ctx context.Context, evidence string) ([]Property, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s/properties.json", url.PathEscape(evidence)), nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Properties struct {
			Property []Property `json:"property"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode properties %q: %w", evidence, err)
	}
	return env.Properties.Property, nil
}

// GetSamples returns up to `limit` rows of `evidence` with full detail.
func (c *Client) GetSamples(ctx context.Context, evidence string, limit int) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("detail", "full")
	body, err := c.get(ctx, fmt.Sprintf("%s.json", url.PathEscape(evidence)), q)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode samples %q: %w", evidence, err)
	}
	w, ok := raw["winstrom"].(map[string]any)
	if !ok {
		return nil, nil
	}
	rows, ok := w[evidence].([]any)
	if !ok {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// Count returns the row count of one evidence.
func (c *Client) Count(ctx context.Context, evidence string) (int64, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s/$count", url.PathEscape(evidence)), nil)
	if err != nil {
		return 0, err
	}
	var env struct {
		Winstrom map[string]any `json:"winstrom"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, fmt.Errorf("decode count %q: %w", evidence, err)
	}
	if s, ok := env.Winstrom["@rowCount"].(string); ok {
		n, _ := strconv.ParseInt(s, 10, 64)
		return n, nil
	}
	return 0, nil
}
```

- [ ] **Step 5: Run test, verify it passes**

```bash
go test ./internal/connectors/catalog/flexi/...
```

Expected: PASS. Also `go build ./...` should now pass.

- [ ] **Step 6: Commit**

```bash
git add internal/connectors/catalog/flexi/client.go internal/connectors/catalog/flexi/client_test.go internal/connectors/catalog/flexi/testdata/
git commit -m "feat(flexi): rest client for evidence-list/properties/samples/count"
```

---

## Task 7: Binding repository

**Files:**
- Create: `internal/connectors/discovery/repo.go`
- Test: `internal/connectors/discovery/repo_test.go`

- [ ] **Step 1: Write the failing repo test**

```go
// internal/connectors/discovery/repo_test.go
package discovery

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping db test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestBindingCreateAndGet(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRepo(pool)

	tenantID := ensureTenant(t, pool)

	b, err := repo.Create(ctx, tenantID, "abra-flexi")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.State != StateDiscovering {
		t.Fatalf("state = %q, want discovering", b.State)
	}

	got, err := repo.GetByID(ctx, tenantID, b.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != b.ID {
		t.Fatalf("id mismatch")
	}
}

func TestBindingUpdateMappingAndState(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRepo(pool)
	tenantID := ensureTenant(t, pool)

	b, _ := repo.Create(ctx, tenantID, "abra-flexi")
	art := &Artifact{
		Version: 1, Connector: "abra-flexi", DiscoveredAt: time.Now(),
		Evidences: map[string]*Evidence{"adresar": {Role: RoleCustomer, Confidence: 1}},
	}
	if err := repo.SaveMapping(ctx, b.ID, art); err != nil {
		t.Fatalf("save mapping: %v", err)
	}
	if err := repo.UpdateState(ctx, b.ID, StateProposed, ""); err != nil {
		t.Fatalf("update state: %v", err)
	}
	got, _ := repo.GetByID(ctx, tenantID, b.ID)
	if got.State != StateProposed {
		t.Fatalf("state not updated")
	}
	if got.Mapping == nil || got.Mapping.Evidences["adresar"].Role != RoleCustomer {
		t.Fatalf("mapping not persisted")
	}
}

// ensureTenant inserts (or reuses) a tenant row and returns its id; lives next
// to the test so the repo test is self-contained.
func ensureTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO openrow.tenants (slug, name) VALUES ($1, $2)
		 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
		 RETURNING id`,
		"test-discovery", "Test Discovery",
	).Scan(&id)
	if err != nil {
		t.Fatalf("ensure tenant: %v", err)
	}
	return id
}
```

- [ ] **Step 2: Run test, expect skip when no DB or fail otherwise**

```bash
go test ./internal/connectors/discovery/...
```

Expected: FAIL — `NewRepo` undefined.

- [ ] **Step 3: Implement the repo**

```go
// internal/connectors/discovery/repo.go
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type State string

const (
	StateDiscovering State = "discovering"
	StateProposed    State = "proposed"
	StateActive      State = "active"
	StateError       State = "error"
)

type Binding struct {
	ID                string
	TenantID          string
	ConnectorID       string
	State             State
	Mapping           *Artifact
	LLMClassification bool
	LastError         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

var ErrNotFound = errors.New("binding not found")

func (r *Repo) Create(ctx context.Context, tenantID, connectorID string) (*Binding, error) {
	var b Binding
	err := r.pool.QueryRow(ctx, `
		INSERT INTO openrow.external_bindings (tenant_id, connector_id, state)
		VALUES ($1, $2, 'discovering')
		ON CONFLICT (tenant_id, connector_id) DO UPDATE
		   SET state = 'discovering', last_error = NULL, updated_at = now()
		RETURNING id, tenant_id, connector_id, state, llm_classification, created_at, updated_at`,
		tenantID, connectorID,
	).Scan(&b.ID, &b.TenantID, &b.ConnectorID, &b.State, &b.LLMClassification, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repo) GetByID(ctx context.Context, tenantID, bindingID string) (*Binding, error) {
	var (
		b   Binding
		raw []byte
		lerr *string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, connector_id, state, mapping, llm_classification,
		       last_error, created_at, updated_at
		FROM openrow.external_bindings
		WHERE id = $1 AND tenant_id = $2`,
		bindingID, tenantID,
	).Scan(&b.ID, &b.TenantID, &b.ConnectorID, &b.State, &raw, &b.LLMClassification,
		&lerr, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lerr != nil {
		b.LastError = *lerr
	}
	if len(raw) > 0 {
		var art Artifact
		if err := json.Unmarshal(raw, &art); err != nil {
			return nil, fmt.Errorf("decode mapping: %w", err)
		}
		b.Mapping = &art
	}
	return &b, nil
}

func (r *Repo) GetByConnector(ctx context.Context, tenantID, connectorID string) (*Binding, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM openrow.external_bindings WHERE tenant_id = $1 AND connector_id = $2`,
		tenantID, connectorID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, tenantID, id)
}

func (r *Repo) SaveMapping(ctx context.Context, bindingID string, art *Artifact) error {
	raw, err := json.Marshal(art)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE openrow.external_bindings SET mapping = $2, updated_at = now() WHERE id = $1`,
		bindingID, raw)
	return err
}

func (r *Repo) UpdateState(ctx context.Context, bindingID string, state State, lastErr string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE openrow.external_bindings
		SET state = $2, last_error = NULLIF($3, ''), updated_at = now()
		WHERE id = $1`,
		bindingID, state, lastErr)
	return err
}

type ReviewItem struct {
	ID         string          `json:"id"`
	BindingID  string          `json:"binding_id"`
	Evidence   string          `json:"evidence"`
	Field      string          `json:"field,omitempty"`
	Proposed   json.RawMessage `json:"proposed"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty"`
}

func (r *Repo) ReplaceReviewItems(ctx context.Context, bindingID string, items []ReviewItem) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`DELETE FROM openrow.external_binding_review_items WHERE binding_id = $1 AND status = 'pending'`,
		bindingID); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO openrow.external_binding_review_items (binding_id, evidence, field, proposed)
			VALUES ($1, $2, NULLIF($3,''), $4)`,
			bindingID, it.Evidence, it.Field, []byte(it.Proposed)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repo) ListReviewItems(ctx context.Context, bindingID string) ([]ReviewItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, binding_id, evidence, COALESCE(field, ''), proposed, status, created_at, resolved_at
		FROM openrow.external_binding_review_items
		WHERE binding_id = $1
		ORDER BY status, created_at`, bindingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		var it ReviewItem
		var raw []byte
		if err := rows.Scan(&it.ID, &it.BindingID, &it.Evidence, &it.Field, &raw,
			&it.Status, &it.CreatedAt, &it.ResolvedAt); err != nil {
			return nil, err
		}
		it.Proposed = raw
		out = append(out, it)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test against a live DB, verify it passes**

```bash
make db-up
DATABASE_URL="$(grep ^DATABASE_URL .env | cut -d= -f2-)" go test ./internal/connectors/discovery/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/discovery/repo.go internal/connectors/discovery/repo_test.go
git commit -m "feat(discovery): binding repository"
```

---

## Task 8: Stage 5 — LLM classify

**Files:**
- Create: `internal/connectors/discovery/llm.go`
- Create: `internal/connectors/discovery/llm_test.go`
- Create: `internal/connectors/catalog/flexi/prompts.go`

- [ ] **Step 1: Write the failing LLM-classifier test**

```go
// internal/connectors/discovery/llm_test.go
package discovery

import (
	"context"
	"encoding/json"
	"testing"
)

// stubClassifier returns canned JSON so tests don't depend on an LLM.
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
```

- [ ] **Step 2: Run test, verify it fails**

Expected: FAIL — `ClassifyEvidence`, `Classifier`, `ClassifyInput` undefined.

- [ ] **Step 3: Implement the classifier**

```go
// internal/connectors/discovery/llm.go
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

// ClassifyInput carries everything Stage 5 sends to the LLM.
type ClassifyInput struct {
	EvidenceName string
	Properties   json.RawMessage    // raw properties JSON snippet
	Samples      []map[string]any   // 0-N sample rows
	HasSamples   bool
}

// ClassifyOutput is the structured result of one Stage 5 call.
type ClassifyOutput struct {
	Role       Role               `json:"role"`
	Confidence float64            `json:"confidence"`
	Reasoning  string             `json:"reasoning,omitempty"`
	FieldMap   map[string]FieldHint `json:"field_map,omitempty"`
}

type FieldHint struct {
	Role       Role    `json:"role"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// ClassifyEvidence calls the classifier and parses its JSON reply. Confidence
// is capped at 0.7 when HasSamples is false (see spec, Stage 5 confidence cap).
func ClassifyEvidence(ctx context.Context, c Classifier, in ClassifyInput) (*ClassifyOutput, error) {
	prompt, err := buildEvidencePrompt(in)
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
		return nil, fmt.Errorf("decode classifier reply: %w (raw=%s)", err, truncate(raw, 200))
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

// buildEvidencePrompt assembles the per-evidence Stage-5 prompt. The actual
// Flexi-aware prompt template lives in catalog/flexi/prompts.go and is wired
// in by the service. This package-level fallback is the generic shape.
var buildEvidencePrompt = func(in ClassifyInput) (string, error) {
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

// extractJSONBlock pulls JSON out of a reply that may have fences or prose.
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
```

```go
// internal/connectors/catalog/flexi/prompts.go
package flexi

import (
	"encoding/json"
	"fmt"

	"github.com/openrow/openrow/internal/connectors/discovery"
)

// BuildPrompt is the Flexi-aware evidence classification prompt. Wired into
// discovery.Service.LLMPromptBuilder when discovery is constructed for Flexi.
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
```

- [ ] **Step 4: Run test, verify it passes**

```bash
go test ./internal/connectors/discovery/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/discovery/llm.go internal/connectors/discovery/llm_test.go internal/connectors/catalog/flexi/prompts.go
git commit -m "feat(discovery): stage 5 llm classifier with confidence cap"
```

---

## Task 9: Stages — tier, persist, propose

**Files:**
- Create: `internal/connectors/discovery/stages.go`
- Create: `internal/connectors/discovery/stages_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/connectors/discovery/stages_test.go
package discovery

import "testing"

func TestTier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		conf       float64
		source     Source
		userCustom bool
		want       Review
	}{
		{1.0, SourceHeuristic, false, ReviewAuto},
		{0.95, SourceLLM, false, ReviewAuto},
		{0.85, SourceLLM, false, ReviewAutoLowConfidence},
		{0.6, SourceLLM, false, ReviewNeedsReview},
		{0.99, SourceLLM, true, ReviewNeedsReview}, // user-defined custom field always review
	}
	for _, tc := range cases {
		got := Tier(tc.conf, tc.source, tc.userCustom)
		if got != tc.want {
			t.Errorf("Tier(%v,%v,%v) = %v, want %v", tc.conf, tc.source, tc.userCustom, got, tc.want)
		}
	}
}

func TestProposeEntities(t *testing.T) {
	t.Parallel()
	art := &Artifact{
		Evidences: map[string]*Evidence{
			"adresar":        {Role: RoleCustomer, Confidence: 1.0, Review: ReviewAuto},
			"faktura-vydana": {Role: RoleInvoiceOutgoing, Confidence: 1.0, Review: ReviewAuto},
		},
	}
	ProposeEntities(art)
	if art.Evidences["adresar"].OpenRowEntity == nil {
		t.Errorf("customer should get an entity proposal")
	}
	if art.Evidences["faktura-vydana"].OpenRowEntity != nil {
		t.Errorf("invoice (transactional) should not be promoted")
	}
}

func TestBuildReviewItems(t *testing.T) {
	t.Parallel()
	art := &Artifact{
		Evidences: map[string]*Evidence{
			"weird": {Role: RoleUnknown, Confidence: 0.4, Review: ReviewNeedsReview,
				Fields: map[string]*Field{
					"foo": {Role: RoleUnknown, Confidence: 0.3, Review: ReviewNeedsReview},
					"bar": {Role: RoleDisplayName, Confidence: 1.0, Review: ReviewAuto},
				}},
			"adresar": {Role: RoleCustomer, Confidence: 1.0, Review: ReviewAuto,
				Fields: map[string]*Field{
					"userField1@FX001": {Role: RoleUnknown, Confidence: 0.5, Review: ReviewNeedsReview},
				}},
		},
	}
	items := BuildReviewItems(art)
	// Expect: weird (evidence-level), weird.foo, adresar.userField1
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (%+v)", len(items), items)
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement stages**

```go
// internal/connectors/discovery/stages.go
package discovery

import "encoding/json"

// Tier maps (confidence, source, userCustom) to a review tier. userCustom
// covers Flexi-style customer-defined fields (userField*) which always land
// in the review queue regardless of confidence — see spec Stage 6.
func Tier(confidence float64, source Source, userCustom bool) Review {
	if userCustom {
		return ReviewNeedsReview
	}
	switch {
	case confidence >= 0.95:
		return ReviewAuto
	case confidence >= 0.7:
		return ReviewAutoLowConfidence
	default:
		return ReviewNeedsReview
	}
}

// ProposeEntities adds OpenRowEntity proposals for evidences whose role is in
// PromotableRoles. Idempotent: re-running does not stomp existing proposals.
func ProposeEntities(art *Artifact) {
	for _, ev := range art.Evidences {
		if !PromotableRoles[ev.Role] {
			continue
		}
		if ev.OpenRowEntity != nil {
			continue
		}
		ev.OpenRowEntity = &EntityProposal{
			Name:        string(ev.Role),
			DisplayName: defaultDisplayName(ev.Role),
		}
	}
}

func defaultDisplayName(r Role) string {
	switch r {
	case RoleCustomer:
		return "Zákazník"
	case RoleSupplier:
		return "Dodavatel"
	case RoleProduct:
		return "Produkt"
	case RoleProject:
		return "Zakázka"
	case RoleCostCenter:
		return "Středisko"
	case RoleBankAccount:
		return "Bankovní účet"
	}
	return string(r)
}

// BuildReviewItems flattens the artifact into a list of items that need
// human attention. One entry per evidence in needs_review tier, plus one per
// field in needs_review.
func BuildReviewItems(art *Artifact) []ReviewItem {
	out := []ReviewItem{}
	for evName, ev := range art.Evidences {
		if ev.Review == ReviewNeedsReview {
			out = append(out, ReviewItem{
				Evidence: evName,
				Proposed: mustJSON(ev),
				Status:   "pending",
			})
		}
		for fName, f := range ev.Fields {
			if f.Review == ReviewNeedsReview {
				out = append(out, ReviewItem{
					Evidence: evName,
					Field:    fName,
					Proposed: mustJSON(f),
					Status:   "pending",
				})
			}
		}
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
```

- [ ] **Step 4: Run test, verify it passes**

```bash
go test ./internal/connectors/discovery/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/discovery/stages.go internal/connectors/discovery/stages_test.go
git commit -m "feat(discovery): tier, propose entities, build review items"
```

---

## Task 10: Discovery service (orchestrator)

**Files:**
- Create: `internal/connectors/discovery/service.go`
- Test: `internal/connectors/discovery/service_test.go`

- [ ] **Step 1: Write a failing end-to-end test using fakes**

```go
// internal/connectors/discovery/service_test.go
package discovery

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeFetcher provides canned Flexi responses for the orchestrator test.
type fakeFetcher struct {
	evidences []string
	props     map[string][]Property
	samples   map[string][]map[string]any
	counts    map[string]int64
}

func (f *fakeFetcher) Connect(ctx context.Context) (string, error)          { return "2024.5.3", nil }
func (f *fakeFetcher) ListEvidences(ctx context.Context) ([]string, error)  { return f.evidences, nil }
func (f *fakeFetcher) GetProperties(ctx context.Context, e string) ([]Property, error) {
	return f.props[e], nil
}
func (f *fakeFetcher) GetSamples(ctx context.Context, e string, n int) ([]map[string]any, error) {
	return f.samples[e], nil
}
func (f *fakeFetcher) Count(ctx context.Context, e string) (int64, error) { return f.counts[e], nil }

// fakeHeuristic only knows about adresar.
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

// fakeClassifier returns canned JSON.
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
	// vendor_version populated
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
	_ = json.Marshal(art) // ensure marshallable
	_ = time.Now()
}
```

- [ ] **Step 2: Run test, verify it fails**

Expected: FAIL — `Service`, `Fetcher`, `Heuristic` types undefined.

- [ ] **Step 3: Implement the service**

```go
// internal/connectors/discovery/service.go
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

// Service runs the discovery pipeline.
type Service struct {
	Fetcher       Fetcher
	Heuristic     Heuristic
	Classifier    Classifier    // nil disables Stage 5
	PromptBuilder PromptBuilder // nil falls back to discovery.buildEvidencePrompt
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
```

- [ ] **Step 4: Run test, verify it passes**

```bash
go test ./internal/connectors/discovery/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/discovery/service.go internal/connectors/discovery/service_test.go
git commit -m "feat(discovery): stage orchestrator service"
```

---

## Task 11: Workspace LLM adapter

**Files:**
- Create: `internal/connectors/discovery/llm_workspace.go`
- Test: extend `internal/connectors/discovery/llm_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/connectors/discovery/llm_test.go`:

```go
func TestWorkspaceClassifierUsesResolvedConfig(t *testing.T) {
	t.Parallel()
	t.Skip("integration: requires live LLM, run with OPENROW_LIVE_LLM_TESTS=1")
}
```

(This task wires real-LLM calls. Real integration tests are gated; the unit test exists only as a placeholder so the file stays valid Go.)

- [ ] **Step 2: Implement the adapter**

```go
// internal/connectors/discovery/llm_workspace.go
package discovery

import (
	"context"
	"errors"

	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/llm"
)

// WorkspaceClassifier is the production Classifier. It resolves the
// workspace's LLM config (the global setting) on every call and uses it. No
// per-binding model overrides — see spec Stage 5.
type WorkspaceClassifier struct {
	LLM      *llm.Service
	TenantID string
}

func (w *WorkspaceClassifier) Classify(ctx context.Context, prompt string) (string, error) {
	cfg, err := w.LLM.Resolve(ctx, w.TenantID)
	if err != nil {
		return "", err
	}
	client := llm.NewClient(cfg)
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: cfg.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You return JSON only, with no prose, no markdown fences."},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Temperature: 0,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("classifier: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
go test ./internal/connectors/discovery/...
```

Expected: build clean, tests pass (the new test is skipped).

- [ ] **Step 4: Commit**

```bash
git add internal/connectors/discovery/llm_workspace.go internal/connectors/discovery/llm_test.go
git commit -m "feat(discovery): workspace llm classifier adapter"
```

---

## Task 12: Flexi fetcher adapter

**Files:**
- Create: `internal/connectors/catalog/flexi/fetcher.go`
- Test: `internal/connectors/catalog/flexi/fetcher_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/connectors/catalog/flexi/fetcher_test.go
package flexi

import (
	"context"
	"testing"
)

func TestFetcherWrapsClient(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	f, err := NewFetcher(map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "user1", "password": "pw",
	}, true)
	if err != nil {
		t.Fatalf("new fetcher: %v", err)
	}
	v, err := f.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	_ = v // server-version detection requires header; fixture is fine returning empty
	evs, err := f.ListEvidences(context.Background())
	if err != nil || len(evs) != 3 {
		t.Fatalf("evidences: %+v err=%v", evs, err)
	}
	props, err := f.GetProperties(context.Background(), "adresar")
	if err != nil || len(props) != 5 {
		t.Fatalf("props: %+v err=%v", props, err)
	}
	// Property type names match what we feed forward.
	if props[0].Name != "nazev" || props[0].Type != "string" {
		t.Fatalf("first property: %+v", props[0])
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

- [ ] **Step 3: Implement the fetcher**

```go
// internal/connectors/catalog/flexi/fetcher.go
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
```

`discovery.Property` was added in Task 10. No changes to `service.go` here — just verify the fetcher adapter compiles against the existing definition.

- [ ] **Step 4: Run tests, verify they pass**

```bash
go build ./... && go test ./internal/connectors/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/catalog/flexi/fetcher.go internal/connectors/catalog/flexi/fetcher_test.go internal/connectors/discovery/service.go
git commit -m "feat(flexi): fetcher and heuristic adapters for discovery"
```

---

## Task 13: Flexi query Action (agent tool)

**Files:**
- Create: `internal/connectors/catalog/flexi/action_query.go`
- Modify: `internal/connectors/catalog/flexi/flexi.go` (wire actions)
- Test: `internal/connectors/catalog/flexi/action_query_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/connectors/catalog/flexi/action_query_test.go
package flexi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestActionQueryByRole(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	creds := map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "user1", "password": "pw",
	}
	// Inject a resolver that pretends discovery already produced this mapping.
	resolver := stubMappingResolver{
		role: "adresar",
		fieldMap: map[string]string{
			"display_name":       "nazev",
			"registration_id_cz": "ic",
			"vat_id_cz":          "dic",
			"email":              "email",
		},
	}
	in := json.RawMessage(`{"role":"customer","limit":10}`)
	result, err := queryWithResolver(context.Background(), creds, in, resolver, true)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows, ok := result.([]map[string]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %T %+v", result, result)
	}
	if rows[0]["display_name"] != "Acme s.r.o." {
		t.Fatalf("expected mapped display_name, got %+v", rows[0])
	}
}

type stubMappingResolver struct {
	role     string
	fieldMap map[string]string // canonical → vendor
}

func (s stubMappingResolver) Resolve(ctx context.Context, tenantID, connectorID, role string) (string, map[string]string, error) {
	return s.role, s.fieldMap, nil
}
```

- [ ] **Step 2: Run test, verify it fails**

Expected: FAIL — `queryWithResolver` and `MappingResolver` undefined.

- [ ] **Step 3: Implement the action**

```go
// internal/connectors/catalog/flexi/action_query.go
package flexi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/openrow/openrow/internal/connectors"
)

// MappingResolver returns (evidence_name, field_map) for a canonical role on
// (tenant, connector). Implementations live elsewhere — discovery.Repo and a
// service shim that pulls the tenant ID from context.
type MappingResolver interface {
	Resolve(ctx context.Context, tenantID, connectorID, role string) (evidence string, fieldMap map[string]string, err error)
}

// queryWithResolver is the testable inner. The exported Action wraps it with
// the production resolver and tenant context.
func queryWithResolver(ctx context.Context, creds map[string]string, raw json.RawMessage, mr MappingResolver, allowInternal bool) (any, error) {
	var in struct {
		Role   string         `json:"role"`
		Filter map[string]any `json:"filter"`
		Fields []string       `json:"fields"`
		Limit  int            `json:"limit"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if in.Role == "" {
		return nil, errors.New("role is required")
	}
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 100
	}

	tenantID, connectorID := "", ConnectorID
	if v, ok := ctx.Value(tenantKey{}).(string); ok {
		tenantID = v
	}
	evidence, fieldMap, err := mr.Resolve(ctx, tenantID, connectorID, in.Role)
	if err != nil {
		return nil, err
	}
	if evidence == "" {
		return nil, fmt.Errorf("role %q is not mapped on this binding", in.Role)
	}

	client, err := NewClient(creds, allowInternal)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("limit", strconv.Itoa(in.Limit))
	q.Set("detail", "full")
	if where := buildFilter(in.Filter, fieldMap); where != "" {
		q.Set("where", where)
	}
	rows, err := client.GetSamplesQuery(ctx, evidence, q)
	if err != nil {
		return nil, err
	}
	// Rename keys: vendor field name → canonical role.
	reverse := make(map[string]string, len(fieldMap))
	for canonical, vendor := range fieldMap {
		reverse[vendor] = canonical
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		mapped := map[string]any{}
		for k, v := range r {
			if alias, ok := reverse[k]; ok {
				mapped[alias] = v
			} else if len(in.Fields) == 0 {
				mapped[k] = v
			}
		}
		out = append(out, mapped)
	}
	return out, nil
}

func buildFilter(filter map[string]any, fieldMap map[string]string) string {
	if len(filter) == 0 {
		return ""
	}
	clauses := make([]string, 0, len(filter))
	for canonical, raw := range filter {
		vendor, ok := fieldMap[canonical]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			clauses = append(clauses, fmt.Sprintf("%s=%q", vendor, v))
		case bool:
			clauses = append(clauses, fmt.Sprintf("%s=%t", vendor, v))
		case float64:
			clauses = append(clauses, fmt.Sprintf("%s=%v", vendor, v))
		}
	}
	return strings.Join(clauses, " and ")
}

type tenantKey struct{}

// WithTenant attaches the tenant ID to the context for the action handler.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

// resolverFunc lets us register a production resolver at server-construction
// time without forming an import cycle from this package back to httpapi.
var resolverFunc MappingResolver

// SetResolver is called once from main() to wire the discovery repo as the
// production MappingResolver for this connector's actions.
func SetResolver(mr MappingResolver) { resolverFunc = mr }

func queryHandler(ctx context.Context, creds map[string]string, in json.RawMessage) (any, error) {
	if resolverFunc == nil {
		return nil, errors.New("discovery resolver not initialised")
	}
	return queryWithResolver(ctx, creds, in, resolverFunc, false)
}
```

Add the `GetSamplesQuery` helper to client.go (next to GetSamples):

```go
// GetSamplesQuery is GetSamples with arbitrary query params (used by the
// read-through action so callers can pass `where` filters).
func (c *Client) GetSamplesQuery(ctx context.Context, evidence string, q url.Values) ([]map[string]any, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s.json", url.PathEscape(evidence)), q)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	w, _ := raw["winstrom"].(map[string]any)
	rows, _ := w[evidence].([]any)
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
```

Wire actions in `flexi.go`:

```go
func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "query",
			Name:        "Query Flexi",
			Description: "Read rows from a Flexi evidence by canonical role. Use roles like 'customer', 'product', 'invoice_outgoing'. Returns rows with canonical-role keys (mapped from Czech column names).",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role": map[string]any{"type": "string",
						"description": "Canonical semantic role. Must be one mapped on this binding (call list_external_bindings to see roles).",
					},
					"filter": map[string]any{"type": "object",
						"description": "Equality filter; keys are canonical roles (e.g. {\"vat_id_cz\":\"CZ12345678\"}).",
					},
					"fields": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
						"description": "Subset of canonical roles to project. Empty = all auto-mapped fields.",
					},
					"limit": map[string]any{"type": "integer", "description": "Max rows; default 100, max 200."},
				},
				"required": []string{"role"},
			},
			Handler: queryHandler,
		},
	}
}
```

- [ ] **Step 4: Run test, verify it passes**

```bash
go test ./internal/connectors/catalog/flexi/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connectors/catalog/flexi/action_query.go internal/connectors/catalog/flexi/action_query_test.go internal/connectors/catalog/flexi/client.go internal/connectors/catalog/flexi/flexi.go
git commit -m "feat(flexi): query action for agent read-through"
```

---

## Task 14: HTTP endpoints

**Files:**
- Create: `internal/httpapi/external_bindings.go`
- Test: `internal/httpapi/external_bindings_test.go`
- Modify: `internal/httpapi/server.go` (mount routes + accept dependencies)
- Modify: `cmd/server/main.go` (construct discovery service and pass it in)

- [ ] **Step 1: Implement the handlers**

```go
// internal/httpapi/external_bindings.go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/connectors"
	"github.com/openrow/openrow/internal/connectors/catalog/flexi"
	"github.com/openrow/openrow/internal/connectors/discovery"
	"github.com/openrow/openrow/internal/llm"
)

type ExternalBindings struct {
	Repo      *discovery.Repo
	Conns     *connectors.Service
	LLM       *llm.Service
	AllowInt  bool // allow internal addresses for outbound (operator opt-in)
}

func (h *ExternalBindings) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/connectors/external-bindings", h.list)
	mux.HandleFunc("POST /api/connectors/external-bindings", h.create)
	mux.HandleFunc("GET /api/connectors/external-bindings/{id}", h.get)
	mux.HandleFunc("GET /api/connectors/external-bindings/{id}/mapping", h.getMapping)
	mux.HandleFunc("PATCH /api/connectors/external-bindings/{id}/mapping", h.patchMapping)
	mux.HandleFunc("GET /api/connectors/external-bindings/{id}/review-items", h.listReview)
	mux.HandleFunc("POST /api/connectors/external-bindings/{id}/review-items/{item}/resolve", h.resolveReview)
	mux.HandleFunc("POST /api/connectors/external-bindings/{id}/activate", h.activate)
	mux.HandleFunc("POST /api/connectors/external-bindings/{id}/rediscover", h.rediscover)
}

type createBindingIn struct {
	ConnectorID       string            `json:"connector_id"`
	Credentials       map[string]string `json:"credentials"`
	LLMClassification *bool             `json:"llm_classification,omitempty"`
}

func (h *ExternalBindings) list(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.TenantID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "no tenant")
		return
	}
	out, err := h.Repo.ListByTenant(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ExternalBindings) create(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.TenantID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "no tenant")
		return
	}
	var in createBindingIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.ConnectorID != flexi.ConnectorID {
		writeError(w, http.StatusBadRequest, "unsupported connector")
		return
	}
	fields := make(map[string]*string, len(in.Credentials))
	for k, v := range in.Credentials {
		s := v
		fields[k] = &s
	}
	if _, err := h.Conns.Upsert(r.Context(), tenantID, in.ConnectorID, connectors.UpsertInput{Fields: fields}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := h.Repo.Create(r.Context(), tenantID, in.ConnectorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	go h.runDiscovery(context.Background(), tenantID, b, in.Credentials)
	writeJSON(w, http.StatusAccepted, b)
}

func (h *ExternalBindings) runDiscovery(ctx context.Context, tenantID string, b *discovery.Binding, creds map[string]string) {
	fetcher, err := flexi.NewFetcher(creds, h.AllowInt)
	if err != nil {
		_ = h.Repo.UpdateState(ctx, b.ID, discovery.StateError, err.Error())
		return
	}
	svc := &discovery.Service{
		Fetcher:       fetcher,
		Heuristic:     flexi.NewHeuristic(),
		Classifier:    &discovery.WorkspaceClassifier{LLM: h.LLM, TenantID: tenantID},
		PromptBuilder: flexi.BuildPrompt,
	}
	art, items, err := svc.Run(ctx, b)
	if err != nil {
		_ = h.Repo.UpdateState(ctx, b.ID, discovery.StateError, err.Error())
		return
	}
	if err := h.Repo.SaveMapping(ctx, b.ID, art); err != nil {
		_ = h.Repo.UpdateState(ctx, b.ID, discovery.StateError, err.Error())
		return
	}
	if err := h.Repo.ReplaceReviewItems(ctx, b.ID, items); err != nil {
		_ = h.Repo.UpdateState(ctx, b.ID, discovery.StateError, err.Error())
		return
	}
	_ = h.Repo.UpdateState(ctx, b.ID, discovery.StateProposed, "")
}

func (h *ExternalBindings) get(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := auth.TenantID(r.Context())
	b, err := h.Repo.GetByID(r.Context(), tenantID, r.PathValue("id"))
	if errors.Is(err, discovery.ErrNotFound) {
		writeError(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *ExternalBindings) getMapping(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := auth.TenantID(r.Context())
	b, err := h.Repo.GetByID(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b.Mapping)
}

func (h *ExternalBindings) patchMapping(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := auth.TenantID(r.Context())
	b, err := h.Repo.GetByID(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	var patch struct {
		Evidence string             `json:"evidence"`
		Field    string             `json:"field"`
		Role     discovery.Role     `json:"role"`
		Mirror   *bool              `json:"mirror,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if b.Mapping == nil {
		writeError(w, http.StatusBadRequest, "no mapping yet")
		return
	}
	ev := b.Mapping.Evidences[patch.Evidence]
	if ev == nil {
		writeError(w, http.StatusBadRequest, "unknown evidence")
		return
	}
	if patch.Field != "" {
		f := ev.Fields[patch.Field]
		if f == nil {
			writeError(w, http.StatusBadRequest, "unknown field")
			return
		}
		if patch.Role != "" {
			f.Role = patch.Role
			f.Source = discovery.SourceUser
			f.Confidence = 1.0
			f.Review = discovery.ReviewAuto
		}
	} else {
		if patch.Role != "" {
			ev.Role = patch.Role
			ev.Source = discovery.SourceUser
			ev.Confidence = 1.0
			ev.Review = discovery.ReviewAuto
		}
		if patch.Mirror != nil {
			ev.Mirror = *patch.Mirror
		}
	}
	if err := h.Repo.SaveMapping(r.Context(), b.ID, b.Mapping); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b.Mapping.Evidences[patch.Evidence])
}

func (h *ExternalBindings) listReview(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := auth.TenantID(r.Context())
	bindingID := r.PathValue("id")
	if _, err := h.Repo.GetByID(r.Context(), tenantID, bindingID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	items, err := h.Repo.ListReviewItems(r.Context(), bindingID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ExternalBindings) resolveReview(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := auth.TenantID(r.Context())
	bindingID := r.PathValue("id")
	itemID := r.PathValue("item")
	if _, err := h.Repo.GetByID(r.Context(), tenantID, bindingID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	var in struct {
		Action string         `json:"action"` // accept | edit | reject
		Patch  map[string]any `json:"patch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Repo.ResolveReviewItem(r.Context(), itemID, in.Action); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *ExternalBindings) activate(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := auth.TenantID(r.Context())
	bindingID := r.PathValue("id")
	b, err := h.Repo.GetByID(r.Context(), tenantID, bindingID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if b.State != discovery.StateProposed {
		writeError(w, http.StatusBadRequest, "binding is not in 'proposed' state")
		return
	}
	if err := h.Repo.UpdateState(r.Context(), b.ID, discovery.StateActive, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "active"})
}

func (h *ExternalBindings) rediscover(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := auth.TenantID(r.Context())
	b, err := h.Repo.GetByID(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	cfg, err := h.Conns.Get(r.Context(), tenantID, b.ConnectorID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "credentials missing")
		return
	}
	_ = h.Repo.UpdateState(r.Context(), b.ID, discovery.StateDiscovering, "")
	go h.runDiscovery(context.Background(), tenantID, b, cfg.Credentials)
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "discovering"})
}
```

Also add `ListByTenant`, `ResolveReviewItem`, and the mapping-resolver implementation to `repo.go`:

```go
func (r *Repo) ListByTenant(ctx context.Context, tenantID string) ([]*Binding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM openrow.external_bindings WHERE tenant_id = $1 ORDER BY created_at DESC`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]*Binding, 0, len(ids))
	for _, id := range ids {
		b, err := r.GetByID(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}
```

```go
func (r *Repo) ResolveReviewItem(ctx context.Context, itemID, action string) error {
	switch action {
	case "accept", "edit", "reject":
	default:
		return fmt.Errorf("invalid action %q", action)
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE openrow.external_binding_review_items
		SET status = $2, resolved_at = now()
		WHERE id = $1`, itemID, action)
	return err
}

// ResolveRole returns (evidence, field_map) for a canonical role on
// (tenant, connector). Implements flexi.MappingResolver. Errors if the
// binding is not in 'active' state.
func (r *Repo) ResolveRole(ctx context.Context, tenantID, connectorID, role string) (string, map[string]string, error) {
	b, err := r.GetByConnector(ctx, tenantID, connectorID)
	if err != nil {
		return "", nil, err
	}
	if b.State != StateActive {
		return "", nil, fmt.Errorf("binding state is %q, not active", b.State)
	}
	for name, ev := range b.Mapping.Evidences {
		if string(ev.Role) == role {
			fm := map[string]string{}
			for fname, f := range ev.Fields {
				if f.Role != RoleUnknown {
					fm[string(f.Role)] = fname
				}
			}
			return name, fm, nil
		}
	}
	return "", nil, fmt.Errorf("role %q not mapped", role)
}
```

- [ ] **Step 2: Wire into server.go and main.go**

In `internal/httpapi/server.go`, accept the new dependency on the server struct (look for the existing fields — e.g. `Conns *connectors.Service`) and add:

```go
ExternalBindings *ExternalBindings
```

In its `routes()` (or wherever existing routes mount), call:

```go
if s.ExternalBindings != nil {
	s.ExternalBindings.Mount(mux)
}
```

In `cmd/server/main.go`, near where `connectors.NewService` is constructed, add:

```go
discoveryRepo := discovery.NewRepo(pool)
flexi.SetResolver(discoveryRepoResolver{discoveryRepo})
externalBindings := &httpapi.ExternalBindings{
	Repo:     discoveryRepo,
	Conns:    connectorsSvc,
	LLM:      llmSvc,
	AllowInt: false, // operators flip via OPENROW_ALLOW_INTERNAL_URLS env if needed
}
// then pass externalBindings into the server constructor
```

And a tiny adapter type (in main.go):

```go
// discoveryRepoResolver adapts *discovery.Repo to flexi.MappingResolver by
// pulling tenant ID from the action's context (set by the tool runner).
type discoveryRepoResolver struct{ repo *discovery.Repo }

func (d discoveryRepoResolver) Resolve(ctx context.Context, tenantID, connectorID, role string) (string, map[string]string, error) {
	return d.repo.ResolveRole(ctx, tenantID, connectorID, role)
}
```

- [ ] **Step 3: Run the build and unit tests**

```bash
go build ./...
go test ./internal/...
```

Expected: PASS.

- [ ] **Step 4: Smoke test the endpoints with curl**

```bash
make db-up
make seed
make api  # in a separate terminal — runs the Go server on :8080

# log in to get a session cookie (replace creds with seed user)
curl -c /tmp/cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"demo@openrow.local","password":"openrow123"}'

# create a binding (use a real Flexi demo URL; demo.flexibee.eu works)
curl -b /tmp/cookies.txt -X POST http://localhost:8080/api/connectors/external-bindings \
  -H 'content-type: application/json' \
  -d '{"connector_id":"abra-flexi","credentials":{"base_url":"https://demo.flexibee.eu","company":"demo_s_r_o_","username":"winstrom","password":"winstrom"}}'
# returns binding id, state=discovering

# poll until state=proposed
curl -b /tmp/cookies.txt http://localhost:8080/api/connectors/external-bindings/<id>
```

Expected: state transitions from `discovering` to `proposed` within ~30s. Mapping populated with adresar=customer and friends.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/external_bindings.go internal/httpapi/server.go internal/connectors/discovery/repo.go cmd/server/main.go
git commit -m "feat(httpapi): external bindings endpoints and discovery wiring"
```

---

## Task 15: Frontend — list + connect form

**Files:**
- Create: `web/src/routes/_app.settings.connectors.external.tsx`

- [ ] **Step 1: Add the route**

```tsx
// web/src/routes/_app.settings.connectors.external.tsx
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "@/lib/api";

export const Route = createFileRoute("/_app/settings/connectors/external")({
  component: ExternalConnectorsPage,
});

type Binding = {
  id: string;
  connector_id: string;
  state: "discovering" | "proposed" | "active" | "error";
  last_error?: string;
};

function ExternalConnectorsPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["external-bindings"],
    queryFn: () => api.get<Binding[]>("/api/connectors/external-bindings"),
  });

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-semibold">External (ERP) connectors</h1>
      <ConnectForm />
      <section>
        <h2 className="mt-6 text-lg font-medium">Bindings</h2>
        {isLoading && <p>Loading…</p>}
        <ul className="divide-y">
          {(data ?? []).map((b) => (
            <li key={b.id} className="flex items-center gap-4 py-2">
              <span className="font-mono text-sm">{b.connector_id}</span>
              <span className="text-sm text-muted-foreground">{b.state}</span>
              <Link
                className="ml-auto text-sm underline"
                to="/_app/settings/connectors/external/$id"
                params={{ id: b.id }}
              >
                Open
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

function ConnectForm() {
  const [baseUrl, setBaseUrl] = useState("");
  const [company, setCompany] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const queryClient = useQueryClient();
  const nav = useNavigate();
  const m = useMutation({
    mutationFn: async () =>
      api.post<Binding>("/api/connectors/external-bindings", {
        connector_id: "abra-flexi",
        credentials: { base_url: baseUrl, company, username, password },
      }),
    onSuccess: async (b) => {
      await queryClient.invalidateQueries({ queryKey: ["external-bindings"] });
      nav({ to: "/_app/settings/connectors/external/$id", params: { id: b.id } });
    },
  });

  return (
    <form
      className="grid max-w-lg gap-3 rounded border p-4"
      onSubmit={(e) => {
        e.preventDefault();
        m.mutate();
      }}
    >
      <h2 className="text-lg font-medium">Connect ABRA Flexi</h2>
      <label className="grid gap-1 text-sm">
        Server URL
        <input
          className="rounded border px-2 py-1"
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          placeholder="https://demo.flexibee.eu"
          required
        />
      </label>
      <label className="grid gap-1 text-sm">
        Company
        <input
          className="rounded border px-2 py-1"
          value={company}
          onChange={(e) => setCompany(e.target.value)}
          required
        />
      </label>
      <label className="grid gap-1 text-sm">
        Username
        <input
          className="rounded border px-2 py-1"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          required
        />
      </label>
      <label className="grid gap-1 text-sm">
        Password
        <input
          className="rounded border px-2 py-1"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
      </label>
      {m.error && <p className="text-sm text-red-600">{(m.error as Error).message}</p>}
      <button
        type="submit"
        disabled={m.isPending}
        className="rounded bg-black px-3 py-1 text-sm text-white"
      >
        {m.isPending ? "Connecting…" : "Connect and discover"}
      </button>
    </form>
  );
}
```

- [ ] **Step 2: Verify the route compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/routes/_app.settings.connectors.external.tsx
git commit -m "feat(web): external connectors list and connect form"
```

---

## Task 16: Frontend — status + review screen

**Files:**
- Create: `web/src/routes/_app.settings.connectors.external.$id.tsx`

- [ ] **Step 1: Add the route**

```tsx
// web/src/routes/_app.settings.connectors.external.$id.tsx
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";

export const Route = createFileRoute("/_app/settings/connectors/external/$id")({
  component: BindingDetail,
});

type Binding = {
  id: string;
  state: "discovering" | "proposed" | "active" | "error";
  last_error?: string;
  Mapping?: {
    evidences: Record<
      string,
      {
        role: string;
        confidence: number;
        source: string;
        review: string;
        mirror: boolean;
        row_count: number;
        fields: Record<string, { role: string; confidence: number; review?: string }>;
        openrow_entity?: { name: string; display_name: string };
      }
    >;
  };
};

type ReviewItem = {
  id: string;
  evidence: string;
  field?: string;
  proposed: any;
  status: string;
};

function BindingDetail() {
  const { id } = Route.useParams();
  const qc = useQueryClient();
  const binding = useQuery({
    queryKey: ["binding", id],
    queryFn: () => api.get<Binding>(`/api/connectors/external-bindings/${id}`),
    refetchInterval: (q) => (q.state.data?.state === "discovering" ? 2000 : false),
  });
  const review = useQuery({
    queryKey: ["binding-review", id],
    queryFn: () => api.get<ReviewItem[]>(`/api/connectors/external-bindings/${id}/review-items`),
    enabled: !!binding.data && binding.data.state !== "discovering",
  });
  const activate = useMutation({
    mutationFn: () => api.post(`/api/connectors/external-bindings/${id}/activate`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["binding", id] }),
  });
  const rediscover = useMutation({
    mutationFn: () => api.post(`/api/connectors/external-bindings/${id}/rediscover`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["binding", id] }),
  });
  const resolve = useMutation({
    mutationFn: (itemId: string) =>
      api.post(`/api/connectors/external-bindings/${id}/review-items/${itemId}/resolve`, { action: "accept" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["binding-review", id] }),
  });

  if (binding.isLoading) return <p className="p-6">Loading…</p>;
  const b = binding.data!;
  return (
    <div className="space-y-6 p-6">
      <header>
        <h1 className="text-2xl font-semibold">{b.id}</h1>
        <p className="text-sm text-muted-foreground">state: {b.state}</p>
        {b.last_error && <p className="text-sm text-red-600">{b.last_error}</p>}
      </header>

      <div className="flex gap-2">
        <button
          className="rounded border px-3 py-1 text-sm"
          onClick={() => rediscover.mutate()}
          disabled={rediscover.isPending}
        >
          Re-discover
        </button>
        <button
          className="rounded bg-black px-3 py-1 text-sm text-white disabled:opacity-50"
          onClick={() => activate.mutate()}
          disabled={b.state !== "proposed" || activate.isPending}
        >
          Activate
        </button>
      </div>

      {b.Mapping && (
        <section>
          <h2 className="mb-2 text-lg font-medium">Mapping</h2>
          <table className="w-full text-sm">
            <thead className="text-left text-xs uppercase text-muted-foreground">
              <tr>
                <th>Evidence</th>
                <th>Role</th>
                <th>Confidence</th>
                <th>Review</th>
                <th>Mirror</th>
                <th>Rows</th>
              </tr>
            </thead>
            <tbody>
              {Object.entries(b.Mapping.evidences).map(([name, ev]) => (
                <tr key={name} className="border-t">
                  <td className="font-mono">{name}</td>
                  <td>{ev.role}</td>
                  <td>{ev.confidence.toFixed(2)}</td>
                  <td>{ev.review}</td>
                  <td>{ev.mirror ? "yes" : "no"}</td>
                  <td>{ev.row_count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {review.data && review.data.length > 0 && (
        <section>
          <h2 className="mb-2 text-lg font-medium">Review queue ({review.data.length})</h2>
          <ul className="space-y-2">
            {review.data.map((it) => (
              <li key={it.id} className="rounded border p-3 text-sm">
                <p>
                  <span className="font-mono">{it.evidence}</span>
                  {it.field && <span> · {it.field}</span>}
                </p>
                <pre className="mt-1 overflow-x-auto rounded bg-gray-50 p-2 text-xs">
                  {JSON.stringify(it.proposed, null, 2)}
                </pre>
                {it.status === "pending" && (
                  <button
                    className="mt-2 rounded border px-2 py-1 text-xs"
                    onClick={() => resolve.mutate(it.id)}
                  >
                    Accept proposed
                  </button>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify the route compiles**

```bash
cd web && npx tsc --noEmit
```

Expected: PASS.

- [ ] **Step 3: Manual UI test**

```bash
make dev
# open http://localhost:5173/settings/connectors/external
# connect against demo.flexibee.eu (winstrom/winstrom)
# watch state transition discovering → proposed
# inspect mapping table
# accept any pending review items
# click Activate
```

Expected: full happy path works end-to-end. Then in chat, ask the agent:
"give me 3 customers from Flexi" — it should call `connector.abra-flexi.query` and return mapped rows.

- [ ] **Step 4: Commit**

```bash
git add web/src/routes/_app.settings.connectors.external.\$id.tsx
git commit -m "feat(web): external binding status and review screen"
```

---

## Task 17: End-to-end integration test

**Files:**
- Create: `internal/connectors/discovery/integration_test.go`

- [ ] **Step 1: Write the integration test**

```go
// internal/connectors/discovery/integration_test.go
package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openrow/openrow/internal/connectors/catalog/flexi"
)

// TestDiscoveryEndToEnd runs the full pipeline against a recorded Flexi
// fixture server, exercising heuristics + LLM stub + tier + propose.
func TestDiscoveryEndToEnd(t *testing.T) {
	srv := newFlexiFixture(t)
	defer srv.Close()

	fetcher, err := flexi.NewFetcher(map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "u", "password": "p",
	}, true)
	if err != nil {
		t.Fatalf("fetcher: %v", err)
	}
	svc := &Service{
		Fetcher:    fetcher,
		Heuristic:  flexi.NewHeuristic(),
		Classifier: stubClassifier{resp: `{"role":"unknown","confidence":0.4}`},
	}
	art, items, err := svc.Run(context.Background(),
		&Binding{ConnectorID: flexi.ConnectorID, LLMClassification: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if art.Evidences["adresar"].Role != RoleCustomer {
		t.Fatalf("adresar should be customer")
	}
	if art.Evidences["adresar"].OpenRowEntity == nil {
		t.Fatalf("customer should be promoted-proposed")
	}
	// At least one review item (the LLM-classified unknown evidence).
	if len(items) == 0 {
		t.Fatalf("expected at least one review item")
	}
}

func newFlexiFixture(t *testing.T) *httptest.Server {
	t.Helper()
	dir := "../catalog/flexi/testdata"
	mux := http.NewServeMux()
	mux.HandleFunc("/c/demo/evidence-list.json", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, dir+"/evidence-list.json")
	})
	mux.HandleFunc("/c/demo/", func(w http.ResponseWriter, r *http.Request) {
		// Map: /c/demo/<evidence>/properties.json | <evidence>.json | <evidence>/$count
		switch {
		case r.URL.Path == "/c/demo/adresar/properties.json":
			serveFile(w, dir+"/adresar-properties.json")
		case r.URL.Path == "/c/demo/adresar.json":
			serveFile(w, dir+"/adresar-samples.json")
		case r.URL.Path == "/c/demo/adresar/$count":
			_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"2"}}`))
		case r.URL.Path == "/c/demo/faktura-vydana/properties.json":
			_, _ = w.Write([]byte(`{"properties":{"property":[{"propertyName":"kod","type":"string"}]}}`))
		case r.URL.Path == "/c/demo/faktura-vydana.json":
			_, _ = w.Write([]byte(`{"winstrom":{"faktura-vydana":[]}}`))
		case r.URL.Path == "/c/demo/faktura-vydana/$count":
			_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"0"}}`))
		case r.URL.Path == "/c/demo/cenik/properties.json":
			_, _ = w.Write([]byte(`{"properties":{"property":[{"propertyName":"nazev","type":"string"}]}}`))
		case r.URL.Path == "/c/demo/cenik.json":
			_, _ = w.Write([]byte(`{"winstrom":{"cenik":[]}}`))
		case r.URL.Path == "/c/demo/cenik/$count":
			_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"0"}}`))
		default:
			http.NotFound(w, r)
		}
	})
	return httptest.NewServer(mux)
}

func serveFile(w http.ResponseWriter, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./...
cd web && npx tsc --noEmit
```

Expected: full green.

- [ ] **Step 3: Commit**

```bash
git add internal/connectors/discovery/integration_test.go
git commit -m "test(discovery): end-to-end pipeline against flexi fixture"
```

---

## Definition of done (MVP)

After all tasks complete, the following should hold:

- `go build ./...`, `go vet ./...`, `go test ./...`, `cd web && npx tsc --noEmit` all pass.
- A workspace user can create a Flexi binding via the UI, watch discovery run, accept review items, and activate the binding.
- After activation, an agent prompt like "list 5 customers from Flexi" results in a `connector.abra-flexi.query` tool call returning rows keyed by canonical roles.
- The mapping artifact is persisted JSONB, viewable via API and UI, editable with `source: "user"` semantics preserved on re-discovery.
- Workspace LLM config is the single source of truth for the model used by Stage 5 (no per-binding override).
- All sample data and column names sent to the LLM are gated by the workspace `llm_classification` toggle on the binding (UI surface for the toggle deferred to follow-up plan; default true).

---

## Deferred to Plan 2 (not in this plan)

- **Mirror worker.** Cursor-based polling, upsert into target tables. Schema migration `0016_external_binding_sync_log` if needed.
- **Entity promotion.** Calling `entities.Service` to create real OpenRow entities from `OpenRowEntity` proposals on activation.
- **Sandbox tables.** `erp_<binding_id>_<evidence>` for non-promoted mirrored evidences.
- **Drift handling.** Re-discovery for one evidence when properties change between syncs.
- **Read-through cache.** 60-second per-query-signature cache.
- **Polished UI.** Tabs (auto-mapped vs low-confidence vs needs-review), per-field edit form, per-evidence mirror toggle, `llm_classification` switch, sync frequency picker.
- **List endpoint.** `GET /api/connectors/external-bindings` (the frontend assumes it; for MVP, return the single binding inline). This is small but cuts both ways — include in this plan if scope allows, otherwise add as a tiny Plan-2 task.

---

## Self-review

**Spec coverage:**
- Runtime model C (hybrid mirror + read-through): Plan 1 covers read-through only. Mirror deferred to Plan 2. Documented.
- Data placement (hybrid binding): bindings live in new tables. Promotion deferred. Documented.
- Trust model (tiered): Tier() function in stages.go applies confidence-based tiering. Custom-field rule covered via `isUserCustomField`.
- Structured pipeline (B): stages 1-7 covered. Mirror (analogous to stages 9+) deferred.
- Workspace LLM only: WorkspaceClassifier resolves on every call; no per-binding model override.
- No-LLM mode: `llm_classification` boolean on the binding; service skips Stage 5 when false.
- SSRF guard: ssrf.go, validated on every NewClient call.
- Empty-evidence: Stage 3 skips samples if count==0; Stage 5 caps confidence at 0.7.
- Entity collision: deferred. Promotion is in Plan 2; collision handling lives there.
- Adresar polymorphism: spec out-of-scope; heuristic maps adresar→customer only.
- Re-discovery user-edit preservation: Artifact.Merge keeps SourceUser entries.
- Promoted entity editability: deferred (no promotion in MVP).

**Placeholder scan:** No TBD/TODO. All steps have concrete code and commands.

**Type consistency:** `discovery.Role`, `discovery.Source`, `discovery.Review`, `discovery.Binding`, `discovery.ReviewItem`, `discovery.Artifact`, `discovery.Evidence`, `discovery.Field`, `discovery.Property` are referenced consistently. The `flexi.Property` (vendor type with `IsWritable`) is translated to `discovery.Property` (with `Writable`) in the fetcher adapter.

**Known gaps to verify during execution:**

1. **`auth.TenantID(ctx)` signature.** Task 14 calls this; verify `internal/auth/context.go` exports it with that exact name and signature. If it's named differently (e.g. `auth.FromContext`), substitute throughout. Grep `internal/auth/` first thing.

2. **Tenant ID propagation to Action handlers.** Task 13's `queryHandler` reads tenant ID from context. The existing `internal/connectors` framework calls `ActionHandler(ctx, creds, input)` — the dispatcher must put tenant ID into ctx before calling the handler, since `creds` only carries credentials, not identity. Read `internal/ai/tools.go` (where the agent dispatches connector actions) and verify the context passed in carries tenant ID. If it doesn't, add it at the dispatch point. This is a real prerequisite — without it, `MappingResolver.Resolve` has no tenant to scope by.

3. **`make api` vs `make dev`.** The plan uses `make dev` for the integrated smoke test; if the engineer prefers split terminals, `make api` and `make web` work the same.
