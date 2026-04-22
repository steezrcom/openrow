package templates

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/reports"
)

func init() {
	Register(&Template{
		ID:          "agency",
		Name:        "Agency",
		Description: "Clients, projects, time tracking, invoices, bank feeds, cost categorisation and budgets for creative and consulting agencies. Czech-flavoured (IČO, DIČ, VAT modes, VS/KS/SS symbols). Apply to an empty workspace.",
		Install:     installAgency,
		FlowSeeds:   agencyFlowSeeds(),
	})
}

func installAgency(ctx context.Context, tenantID, pgSchema string, ents *entities.Service, reps *reports.Service) error {
	// Order matters: each entity's reference fields must point to an
	// already-created entity (or itself). cost_categories before
	// bank_transactions, bank_accounts before bank_transactions.
	specs := []entities.EntitySpec{
		clientsSpec(),
		suppliersSpec(),
		projectsSpec(),
		tasksSpec(),
		invoicesSpec(),
		invoiceItemsSpec(),
		timeEntriesSpec(),
		costCategoriesSpec(),
		bankAccountsSpec(),
		bankTransactionsSpec(),
		budgetsSpec(),
	}
	for _, spec := range specs {
		if _, err := ents.Create(ctx, tenantID, pgSchema, &spec); err != nil {
			return fmt.Errorf("create %s: %w", spec.Name, err)
		}
	}
	if _, err := reps.Create(ctx, tenantID, agencyOverviewDashboard()); err != nil {
		return fmt.Errorf("create agency overview dashboard: %w", err)
	}
	if _, err := reps.Create(ctx, tenantID, financeDashboard()); err != nil {
		return fmt.Errorf("create finance dashboard: %w", err)
	}
	return nil
}

func clientsSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "clients",
		DisplayName: "Klienti",
		Description: "Zákazníci, kterým fakturujete.",
		Fields: []entities.FieldSpec{
			{Name: "name", DisplayName: "Název", DataType: entities.TypeText, IsRequired: true},
			{Name: "ico", DisplayName: "IČO", DataType: entities.TypeText},
			{Name: "dic", DisplayName: "DIČ", DataType: entities.TypeText},
			{Name: "email", DisplayName: "E-mail", DataType: entities.TypeText},
			{Name: "phone", DisplayName: "Telefon", DataType: entities.TypeText},
			{Name: "billing_address", DisplayName: "Fakturační adresa", DataType: entities.TypeText},
			{Name: "vat_mode", DisplayName: "Režim DPH", DataType: entities.TypeText},
			{Name: "currency", DisplayName: "Měna", DataType: entities.TypeText},
			{Name: "payment_terms_days", DisplayName: "Splatnost (dny)", DataType: entities.TypeInteger},
			{Name: "notes", DisplayName: "Poznámky", DataType: entities.TypeText},
		},
	}
}

func suppliersSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "suppliers",
		DisplayName: "Dodavatelé",
		Description: "Freelanceři a dodavatelé, kterým platíte.",
		Fields: []entities.FieldSpec{
			{Name: "name", DisplayName: "Jméno", DataType: entities.TypeText, IsRequired: true},
			{Name: "email", DisplayName: "E-mail", DataType: entities.TypeText},
			{Name: "phone", DisplayName: "Telefon", DataType: entities.TypeText},
			{Name: "ico", DisplayName: "IČO", DataType: entities.TypeText},
			{Name: "dic", DisplayName: "DIČ", DataType: entities.TypeText},
			{Name: "subject_type", DisplayName: "Typ", DataType: entities.TypeText},
			{Name: "hourly_rate", DisplayName: "Hodinová sazba", DataType: entities.TypeNumeric},
			{Name: "currency", DisplayName: "Měna", DataType: entities.TypeText},
			{Name: "status", DisplayName: "Stav", DataType: entities.TypeText},
			{Name: "notes", DisplayName: "Poznámky", DataType: entities.TypeText},
		},
	}
}

func projectsSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "projects",
		DisplayName: "Projekty",
		Description: "Klientské projekty a retainery. Propojte se zakázkou, aby fakturace a výkazy seděly.",
		Fields: []entities.FieldSpec{
			{Name: "name", DisplayName: "Název", DataType: entities.TypeText, IsRequired: true},
			{Name: "client", DisplayName: "Klient", DataType: entities.TypeReference, ReferenceEntity: "clients", IsRequired: true},
			{Name: "status", DisplayName: "Stav", DataType: entities.TypeText},
			{Name: "deal_stage", DisplayName: "Stav dealu", DataType: entities.TypeText, Description: "proposed | confirmed | order_issued | advance_invoiced | advance_paid | delivered | final_invoiced | closed"},
			{Name: "budget_type", DisplayName: "Typ rozpočtu", DataType: entities.TypeText},
			{Name: "budget", DisplayName: "Rozpočet", DataType: entities.TypeNumeric},
			{Name: "advance_pct", DisplayName: "Záloha (%)", DataType: entities.TypeNumeric},
			{Name: "hourly_rate", DisplayName: "Hodinová sazba", DataType: entities.TypeNumeric},
			{Name: "billable", DisplayName: "Fakturovat", DataType: entities.TypeBoolean},
			{Name: "start_date", DisplayName: "Začátek", DataType: entities.TypeDate},
			{Name: "end_date", DisplayName: "Konec", DataType: entities.TypeDate},
			{Name: "description", DisplayName: "Popis", DataType: entities.TypeText},
		},
	}
}

func tasksSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "tasks",
		DisplayName: "Úkoly",
		Description: "Dílčí úkoly v rámci projektů.",
		Fields: []entities.FieldSpec{
			{Name: "name", DisplayName: "Název", DataType: entities.TypeText, IsRequired: true},
			{Name: "project", DisplayName: "Projekt", DataType: entities.TypeReference, ReferenceEntity: "projects", IsRequired: true},
			{Name: "status", DisplayName: "Stav", DataType: entities.TypeText},
			{Name: "assignee", DisplayName: "Řešitel", DataType: entities.TypeText},
			{Name: "due_date", DisplayName: "Termín", DataType: entities.TypeDate},
			{Name: "estimated_hours", DisplayName: "Odhad (h)", DataType: entities.TypeNumeric},
			{Name: "billable", DisplayName: "Fakturovat", DataType: entities.TypeBoolean},
		},
	}
}

func invoicesSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "invoices",
		DisplayName: "Faktury",
		Description: "Objednávkové listy, zálohové a konečné faktury pro klienty. kind rozlišuje typ dokumentu.",
		Fields: []entities.FieldSpec{
			{Name: "number", DisplayName: "Číslo", DataType: entities.TypeText, IsRequired: true, IsUnique: true},
			{Name: "kind", DisplayName: "Typ", DataType: entities.TypeText, Description: "order_sheet | proforma | invoice"},
			{Name: "client", DisplayName: "Klient", DataType: entities.TypeReference, ReferenceEntity: "clients", IsRequired: true},
			{Name: "project", DisplayName: "Projekt", DataType: entities.TypeReference, ReferenceEntity: "projects"},
			{Name: "parent_invoice", DisplayName: "Navazuje na", DataType: entities.TypeReference, ReferenceEntity: "invoices", Description: "Konečná faktura odkazuje na zálohovou; zálohová na objednávkový list."},
			{Name: "variable_symbol", DisplayName: "VS", DataType: entities.TypeText},
			{Name: "issue_date", DisplayName: "Datum vystavení", DataType: entities.TypeDate, IsRequired: true},
			{Name: "due_date", DisplayName: "Splatnost", DataType: entities.TypeDate},
			{Name: "payment_date", DisplayName: "Zaplaceno dne", DataType: entities.TypeDate},
			{Name: "status", DisplayName: "Stav", DataType: entities.TypeText, Description: "draft | sent | paid | overdue | cancelled"},
			{Name: "currency", DisplayName: "Měna", DataType: entities.TypeText},
			{Name: "fx_rate", DisplayName: "Kurz k CZK", DataType: entities.TypeNumeric},
			{Name: "subtotal", DisplayName: "Základ", DataType: entities.TypeNumeric},
			{Name: "vat_amount", DisplayName: "DPH", DataType: entities.TypeNumeric},
			{Name: "total", DisplayName: "Celkem", DataType: entities.TypeNumeric},
			{Name: "vat_mode", DisplayName: "Režim DPH", DataType: entities.TypeText},
			{Name: "notes", DisplayName: "Poznámky", DataType: entities.TypeText},
		},
	}
}

func invoiceItemsSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "invoice_items",
		DisplayName: "Fakturační položky",
		Description: "Řádky jednotlivých faktur.",
		Fields: []entities.FieldSpec{
			{Name: "invoice", DisplayName: "Faktura", DataType: entities.TypeReference, ReferenceEntity: "invoices", IsRequired: true},
			{Name: "description", DisplayName: "Popis", DataType: entities.TypeText, IsRequired: true},
			{Name: "quantity", DisplayName: "Množství", DataType: entities.TypeNumeric, IsRequired: true},
			{Name: "unit", DisplayName: "Jednotka", DataType: entities.TypeText},
			{Name: "unit_price", DisplayName: "Cena za jednotku", DataType: entities.TypeNumeric, IsRequired: true},
			{Name: "vat_rate", DisplayName: "Sazba DPH", DataType: entities.TypeNumeric},
			{Name: "subtotal", DisplayName: "Základ", DataType: entities.TypeNumeric},
			{Name: "project", DisplayName: "Projekt", DataType: entities.TypeReference, ReferenceEntity: "projects"},
		},
	}
}

func timeEntriesSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "time_entries",
		DisplayName: "Časové záznamy",
		Description: "Odpracované hodiny na projektech.",
		Fields: []entities.FieldSpec{
			{Name: "project", DisplayName: "Projekt", DataType: entities.TypeReference, ReferenceEntity: "projects", IsRequired: true},
			{Name: "task", DisplayName: "Úkol", DataType: entities.TypeReference, ReferenceEntity: "tasks"},
			{Name: "person", DisplayName: "Kdo", DataType: entities.TypeText, IsRequired: true},
			{Name: "date", DisplayName: "Datum", DataType: entities.TypeDate, IsRequired: true},
			{Name: "hours", DisplayName: "Hodiny", DataType: entities.TypeNumeric, IsRequired: true},
			{Name: "description", DisplayName: "Popis", DataType: entities.TypeText},
			{Name: "billable", DisplayName: "Fakturovat", DataType: entities.TypeBoolean},
			{Name: "rate", DisplayName: "Sazba", DataType: entities.TypeNumeric},
			{Name: "invoice", DisplayName: "Faktura", DataType: entities.TypeReference, ReferenceEntity: "invoices"},
		},
	}
}

func bankAccountsSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "bank_accounts",
		DisplayName: "Bankovní účty",
		Description: "Firemní účty napojené přes konektory (ČS, ČSOB, Fio, Revolut).",
		Fields: []entities.FieldSpec{
			{Name: "label", DisplayName: "Označení", DataType: entities.TypeText, IsRequired: true, Description: "např. 'ČS hlavní' nebo 'Revolut EUR'"},
			{Name: "bank", DisplayName: "Banka", DataType: entities.TypeText, Description: "csas | csob | fio | revolut"},
			{Name: "connector_id", DisplayName: "Konektor", DataType: entities.TypeText, Description: "ID konektoru (csas, csob, fio, revolut)"},
			{Name: "external_id", DisplayName: "ID v bance", DataType: entities.TypeText, Description: "Account UUID / číslo použité konektorem"},
			{Name: "iban", DisplayName: "IBAN", DataType: entities.TypeText},
			{Name: "account_number", DisplayName: "Číslo účtu", DataType: entities.TypeText},
			{Name: "bank_code", DisplayName: "Kód banky", DataType: entities.TypeText},
			{Name: "currency", DisplayName: "Měna", DataType: entities.TypeText},
			{Name: "balance", DisplayName: "Zůstatek", DataType: entities.TypeNumeric},
			{Name: "is_primary", DisplayName: "Hlavní účet", DataType: entities.TypeBoolean, Description: "Označte hlavní provozní účet pro dashboard cash position."},
			{Name: "last_sync_at", DisplayName: "Poslední sync", DataType: entities.TypeTimestampTZ},
		},
	}
}

func bankTransactionsSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "bank_transactions",
		DisplayName: "Bankovní pohyby",
		Description: "Příchozí a odchozí platby stažené z bank. Spárujte s fakturou pomocí VS a přiřaďte nákladovou kategorii.",
		Fields: []entities.FieldSpec{
			{Name: "account", DisplayName: "Účet", DataType: entities.TypeReference, ReferenceEntity: "bank_accounts", IsRequired: true},
			{Name: "external_id", DisplayName: "ID v bance", DataType: entities.TypeText, IsUnique: true, Description: "Jednoznačné ID pohybu z banky. Index slouží k deduplikaci při re-sync."},
			{Name: "booking_date", DisplayName: "Zaúčtováno", DataType: entities.TypeDate, IsRequired: true},
			{Name: "value_date", DisplayName: "Valuta", DataType: entities.TypeDate},
			{Name: "direction", DisplayName: "Směr", DataType: entities.TypeText, Description: "in | out"},
			{Name: "amount", DisplayName: "Částka", DataType: entities.TypeNumeric, IsRequired: true, Description: "Kladná pro příchozí, záporná pro odchozí."},
			{Name: "currency", DisplayName: "Měna", DataType: entities.TypeText},
			{Name: "counterparty", DisplayName: "Protistrana", DataType: entities.TypeText},
			{Name: "counterparty_iban", DisplayName: "IBAN protistrany", DataType: entities.TypeText},
			{Name: "counterparty_account", DisplayName: "Účet protistrany", DataType: entities.TypeText},
			{Name: "variable_symbol", DisplayName: "VS", DataType: entities.TypeText},
			{Name: "constant_symbol", DisplayName: "KS", DataType: entities.TypeText},
			{Name: "specific_symbol", DisplayName: "SS", DataType: entities.TypeText},
			{Name: "description", DisplayName: "Popis", DataType: entities.TypeText},
			{Name: "matched_invoice", DisplayName: "Spárovaná faktura", DataType: entities.TypeReference, ReferenceEntity: "invoices"},
			{Name: "category", DisplayName: "Kategorie", DataType: entities.TypeReference, ReferenceEntity: "cost_categories"},
			{Name: "needs_review", DisplayName: "Vyžaduje kontrolu", DataType: entities.TypeBoolean},
		},
	}
}

func costCategoriesSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "cost_categories",
		DisplayName: "Nákladové kategorie",
		Description: "Základní členění nákladů na personální, produkční, režijní a ostatní. kind určuje, do kterého koláče patří.",
		Fields: []entities.FieldSpec{
			{Name: "name", DisplayName: "Název", DataType: entities.TypeText, IsRequired: true, IsUnique: true},
			{Name: "kind", DisplayName: "Typ", DataType: entities.TypeText, IsRequired: true, Description: "personnel | production | overhead | other | revenue"},
			{Name: "description", DisplayName: "Popis", DataType: entities.TypeText},
		},
	}
}

func budgetsSpec() entities.EntitySpec {
	return entities.EntitySpec{
		Name:        "budgets",
		DisplayName: "Rozpočet",
		Description: "Plánované příjmy a náklady po měsících. Slouží k porovnání plán vs. skutečnost a k výhledu na rok dopředu.",
		Fields: []entities.FieldSpec{
			{Name: "period", DisplayName: "Měsíc", DataType: entities.TypeDate, IsRequired: true, Description: "První den měsíce, kterého se částka týká."},
			{Name: "category", DisplayName: "Kategorie", DataType: entities.TypeReference, ReferenceEntity: "cost_categories", IsRequired: true},
			{Name: "kind", DisplayName: "Typ", DataType: entities.TypeText, IsRequired: true, Description: "income | expense"},
			{Name: "planned_amount", DisplayName: "Plán", DataType: entities.TypeNumeric, IsRequired: true},
			{Name: "currency", DisplayName: "Měna", DataType: entities.TypeText},
			{Name: "notes", DisplayName: "Poznámky", DataType: entities.TypeText},
		},
	}
}

func agencyOverviewDashboard() reports.CreateDashboardInput {
	czk := map[string]interface{}{"number_format": "currency", "currency_code": "CZK"}
	intFmt := map[string]interface{}{"number_format": "integer"}
	decFmt := map[string]interface{}{"number_format": "decimal"}

	asc := &reports.Sort{Field: "label", Dir: "asc"}
	desc := &reports.Sort{Field: "value", Dir: "desc"}

	return reports.CreateDashboardInput{
		Name:             "Přehled agentury",
		Slug:             "agency_overview",
		Description:      "Příjmy, hodiny a otevřené faktury na jednom místě.",
		DefaultDateRange: "mtd",
		Reports:          []reports.CreateReportInput{
			{
				Title:      "Příjmy",
				Subtitle:   "Zaplacené faktury (dle filtru)",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:    "invoices",
					Aggregate: &reports.Aggregate{Fn: reports.AggSum, Field: "total"},
					Filters: []reports.Filter{
						{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"paid"`)},
						{Field: "kind", Op: reports.OpEq, Value: json.RawMessage(`"invoice"`)},
					},
					DateFilterField: "issue_date",
					ComparePeriod:   "previous_period",
				},
				Options: czk,
			},
			{
				Title:      "Po splatnosti",
				Subtitle:   "Neuhrazené po termínu",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:    "invoices",
					Aggregate: &reports.Aggregate{Fn: reports.AggCount},
					Filters:   []reports.Filter{{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"overdue"`)}},
				},
				Options: intFmt,
			},
			{
				Title:      "Hodiny tento týden",
				Subtitle:   "Fakturovatelné i interní",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:          "time_entries",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "hours"},
					DateFilterField: "date",
				},
				Options: decFmt,
			},
			{
				Title:      "Aktivní projekty",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:    "projects",
					Aggregate: &reports.Aggregate{Fn: reports.AggCount},
					Filters:   []reports.Filter{{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"active"`)}},
				},
				Options: intFmt,
			},
			{
				Title:      "Příjmy po měsících",
				Subtitle:   "Zaplacené faktury",
				WidgetType: reports.WidgetLine,
				Width:      8,
				QuerySpec: reports.QuerySpec{
					Entity:    "invoices",
					Aggregate: &reports.Aggregate{Fn: reports.AggSum, Field: "total"},
					GroupBy:   &reports.GroupBy{Field: "issue_date", Bucket: reports.BucketMonth},
					Filters: []reports.Filter{
						{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"paid"`)},
						{Field: "kind", Op: reports.OpEq, Value: json.RawMessage(`"invoice"`)},
					},
					DateFilterField: "issue_date",
					Sort:            asc,
				},
				Options: czk,
			},
			{
				Title:      "Top klienti",
				Subtitle:   "Podle obratu",
				WidgetType: reports.WidgetBar,
				Width:      4,
				QuerySpec: reports.QuerySpec{
					Entity:    "invoices",
					Aggregate: &reports.Aggregate{Fn: reports.AggSum, Field: "total"},
					GroupBy:   &reports.GroupBy{Field: "client"},
					Filters: []reports.Filter{
						{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"paid"`)},
						{Field: "kind", Op: reports.OpEq, Value: json.RawMessage(`"invoice"`)},
					},
					DateFilterField: "issue_date",
					Sort:            desc,
					Limit:           10,
				},
				Options: czk,
			},
		},
	}
}

func financeDashboard() reports.CreateDashboardInput {
	czk := map[string]interface{}{"number_format": "currency", "currency_code": "CZK"}
	intFmt := map[string]interface{}{"number_format": "integer"}

	return reports.CreateDashboardInput{
		Name:             "Finance",
		Slug:             "finance",
		Description:      "Cash flow, nákladové kategorie a plán vs. skutečnost.",
		DefaultDateRange: "mtd",
		Reports:          []reports.CreateReportInput{
			{
				Title:      "Příjmy z banky",
				Subtitle:   "Součet příchozích plateb",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:          "bank_transactions",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "amount"},
					Filters:         []reports.Filter{{Field: "direction", Op: reports.OpEq, Value: json.RawMessage(`"in"`)}},
					DateFilterField: "booking_date",
					ComparePeriod:   "previous_period",
				},
				Options: czk,
			},
			{
				Title:      "Výdaje z banky",
				Subtitle:   "Součet odchozích plateb",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:          "bank_transactions",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "amount"},
					Filters:         []reports.Filter{{Field: "direction", Op: reports.OpEq, Value: json.RawMessage(`"out"`)}},
					DateFilterField: "booking_date",
					ComparePeriod:   "previous_period",
				},
				Options: czk,
			},
			{
				Title:      "Cash position",
				Subtitle:   "Zůstatek napříč účty",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:    "bank_accounts",
					Aggregate: &reports.Aggregate{Fn: reports.AggSum, Field: "balance"},
				},
				Options: czk,
			},
			{
				Title:      "Nespárované platby",
				Subtitle:   "Příchozí bez faktury",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:    "bank_transactions",
					Aggregate: &reports.Aggregate{Fn: reports.AggCount},
					Filters: []reports.Filter{
						{Field: "direction", Op: reports.OpEq, Value: json.RawMessage(`"in"`)},
						{Field: "matched_invoice", Op: reports.OpIsNull},
					},
				},
				Options: intFmt,
			},
			{
				Title:      "Příjmy vs. výdaje",
				Subtitle:   "Po měsících",
				WidgetType: reports.WidgetBar,
				Width:      8,
				QuerySpec: reports.QuerySpec{
					Entity:          "bank_transactions",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "amount"},
					GroupBy:         &reports.GroupBy{Field: "booking_date", Bucket: reports.BucketMonth},
					SeriesBy:        &reports.GroupBy{Field: "direction"},
					DateFilterField: "booking_date",
					Sort:            &reports.Sort{Field: "label", Dir: "asc"},
				},
				Options: map[string]interface{}{"number_format": "currency", "currency_code": "CZK", "stacked": false},
			},
			{
				Title:      "Náklady podle kategorií",
				Subtitle:   "Odchozí platby",
				WidgetType: reports.WidgetPie,
				Width:      4,
				QuerySpec: reports.QuerySpec{
					Entity:    "bank_transactions",
					Aggregate: &reports.Aggregate{Fn: reports.AggSum, Field: "amount"},
					GroupBy:   &reports.GroupBy{Field: "category"},
					Filters: []reports.Filter{
						{Field: "direction", Op: reports.OpEq, Value: json.RawMessage(`"out"`)},
					},
					DateFilterField: "booking_date",
				},
				Options: czk,
			},
			{
				Title:      "Plán příjmů a výdajů",
				Subtitle:   "Rozpočet po měsících (income vs. expense)",
				WidgetType: reports.WidgetBar,
				Width:      12,
				QuerySpec: reports.QuerySpec{
					Entity:          "budgets",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "planned_amount"},
					GroupBy:         &reports.GroupBy{Field: "period", Bucket: reports.BucketMonth},
					SeriesBy:        &reports.GroupBy{Field: "kind"},
					DateFilterField: "period",
					Sort:            &reports.Sort{Field: "label", Dir: "asc"},
				},
				Options: map[string]interface{}{"number_format": "currency", "currency_code": "CZK", "stacked": false},
			},
		},
	}
}

// agencyFlowSeeds defines the built-in automations every Agency workspace
// gets. They ship in dry_run mode so the user can inspect a few runs before
// promoting to auto. Tool allowlists are deliberately tight so an LLM slip
// can't mutate unrelated data.
func agencyFlowSeeds() []FlowSeed {
	return []FlowSeed{
		{
			Name:        "Denní sync banky",
			Description: "Každý den ráno stáhne včerejší transakce ze všech napojených účtů.",
			Goal: `For every row in bank_accounts, fetch yesterday's transactions via the matching connector
(csas, csob, fio or revolut based on the 'connector_id' column) and upsert them into
bank_transactions. Dedupe by external_id: skip transactions whose external_id already exists.
For each new row set account (reference to the bank_accounts row), booking_date, amount with
correct sign (positive for direction='in', negative for 'out'), currency, counterparty, VS/KS/SS
and description. After the run, update bank_accounts.last_sync_at and balance where exposed.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "0 6 * * *"},
			ToolAllowlist: []string{
				"list_entities",
				"query_rows",
				"add_row",
				"update_row",
				"connector_csas_list_accounts",
				"connector_csas_list_transactions",
				"connector_csob_list_accounts",
				"connector_csob_list_transactions",
				"connector_fio_list_transactions",
				"connector_fio_account_info",
				"connector_revolut_list_accounts",
				"connector_revolut_list_transactions",
			},
			Mode:    "dry_run",
			Enabled: true,
		},
		{
			Name:        "Párování plateb s fakturami",
			Description: "Každé ráno po syncu banky spáruje příchozí platby s nezaplacenými fakturami podle VS + částky.",
			Goal: `For every row in bank_transactions where direction='in' AND matched_invoice IS NULL AND
variable_symbol IS NOT NULL, find invoices where variable_symbol equals the transaction's
variable_symbol AND status IN ('sent','overdue') AND total matches the transaction amount
(within 1 CZK tolerance). On match, update the bank_transactions row to set matched_invoice,
and update the invoice to status='paid' with payment_date = transaction booking_date.
If multiple candidates or amount mismatch, set needs_review=true and stop for that row.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "15 6 * * *"},
			ToolAllowlist: []string{"list_entities", "query_rows", "update_row"},
			Mode:          "dry_run",
			Enabled:       true,
		},
		{
			Name:        "Klasifikace nákladů",
			Description: "Každý den rozdělí nekategorizované odchozí platby do kategorií personnel / production / overhead / other.",
			Goal: `For every row in bank_transactions where direction='out' AND category IS NULL,
look up cost_categories and pick the best match based on counterparty, counterparty_iban and
description. Prefer existing categories with a matching 'kind' ('personnel' for salaries/OSVČ
invoices to suppliers; 'production' for project-related subcontractors or materials; 'overhead'
for rent/SaaS/utilities/marketing/legal; 'other' otherwise). If you're unsure, leave the row
with needs_review=true rather than guessing. Use update_row to set category.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "30 6 * * *"},
			ToolAllowlist: []string{"list_entities", "query_rows", "update_row"},
			Mode:          "dry_run",
			Enabled:       true,
		},
		{
			Name:        "Upomínky po splatnosti",
			Description: "Každé pondělí ráno pošle e-mailovou upomínku klientům, jejichž faktury jsou po splatnosti.",
			Goal: `Query invoices where status='overdue' OR (status='sent' AND due_date < today).
For each result, load the client and use connector_resend_send_email to send a polite reminder
in Czech with the invoice number, amount and VS. Also flip the invoice status to 'overdue'
if it isn't already. Skip invoices where a reminder was clearly already sent recently
(check notes for a recent 'reminded on' marker) to avoid spamming.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "0 9 * * 1"},
			ToolAllowlist: []string{
				"list_entities",
				"query_rows",
				"update_row",
				"connector_resend_send_email",
				"connector_slack_post_message",
			},
			Mode:    "approve",
			Enabled: true,
		},
		{
			Name:        "Deal confirmed → vystavit objednávkový list",
			Description: "Když se projekt přepne do stavu 'confirmed', vystaví objednávkový list jako invoice kind='order_sheet'.",
			Goal: `For every project where deal_stage='confirmed' and there is no invoice with
kind='order_sheet' and project=<this project>, create an order_sheet row in invoices.
Use issue_date=today, kind='order_sheet', status='draft', client=project.client,
project=project.id, and fill subtotal/total from project.budget. Generate a unique
number like 'OL-YYYY-NNNN'. After creating, flip project.deal_stage to 'order_issued'.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "*/30 * * * *"},
			ToolAllowlist: []string{"list_entities", "query_rows", "add_row", "update_row"},
			Mode:          "approve",
			Enabled:       true,
		},
		{
			Name:        "Objednávka potvrzena → vystavit zálohovou fakturu",
			Description: "Když je objednávkový list potvrzen, vystaví zálohovou fakturu na procento dané projekt.advance_pct.",
			Goal: `For every project where deal_stage='order_issued', find the related order_sheet invoice.
If the user (or the associated client) has accepted it (look for status='sent' or notes that say
'accepted' / 'potvrzeno'), create a new invoice with kind='proforma', parent_invoice=order_sheet.id,
project=project.id, client=project.client, variable_symbol=generated from project id,
subtotal=project.budget * project.advance_pct / 100, vat and total computed. Status='sent'.
Then update project.deal_stage='advance_invoiced'.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "*/30 * * * *"},
			ToolAllowlist: []string{"list_entities", "query_rows", "add_row", "update_row"},
			Mode:          "approve",
			Enabled:       true,
		},
		{
			Name:        "Projekt dokončen → vystavit doúčtovací fakturu",
			Description: "Když projekt přejde do stavu 'delivered', vystaví konečnou fakturu se zápočtem zaplacené zálohy.",
			Goal: `For every project where deal_stage IN ('advance_paid','delivered') and status='delivered'
and no invoice exists with kind='invoice' for this project, create a final invoice:
kind='invoice', client=project.client, project=project.id, parent_invoice=<proforma.id>,
subtotal = project.budget - proforma.subtotal (i.e. remaining amount), vat/total computed,
variable_symbol derived from project id, issue_date=today, status='sent'.
Set project.deal_stage='final_invoiced'.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "*/30 * * * *"},
			ToolAllowlist: []string{"list_entities", "query_rows", "add_row", "update_row"},
			Mode:          "approve",
			Enabled:       true,
		},
		{
			Name:        "Kontrola kompletní fakturace",
			Description: "Každý pátek ukáže projekty, kde je něco nedofakturovaného (žádná final invoice, přestože projekt je delivered).",
			Goal: `Find every project where deal_stage IN ('delivered','advance_paid') and there is no
invoice with kind='invoice' referencing this project. Post a Slack message with the list,
so the accountant can chase what's missing. Don't create invoices automatically — just report.`,
			TriggerKind:   "cron",
			TriggerConfig: map[string]any{"cron": "0 8 * * 5"},
			ToolAllowlist: []string{"list_entities", "query_rows", "connector_slack_post_message"},
			Mode:          "auto",
			Enabled:       true,
		},
	}
}
