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

	RoleDisplayName      Role = "display_name"
	RoleRegistrationIDCZ Role = "registration_id_cz"
	RoleVATIDCZ          Role = "vat_id_cz"
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
