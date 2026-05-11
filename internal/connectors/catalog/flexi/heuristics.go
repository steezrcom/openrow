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
