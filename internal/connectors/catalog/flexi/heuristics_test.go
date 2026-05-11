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
		"nazev":         discovery.RoleDisplayName,
		"ic":            discovery.RoleRegistrationIDCZ,
		"dic":           discovery.RoleVATIDCZ,
		"email":         discovery.RoleEmail,
		"telefon":       discovery.RolePhone,
		"stitky":        discovery.RoleTags,
		"unknownColumn": discovery.RoleUnknown,
	}
	for field, want := range cases {
		got := FieldRole(field)
		if got != want {
			t.Errorf("FieldRole(%q) = %q, want %q", field, got, want)
		}
	}
}
