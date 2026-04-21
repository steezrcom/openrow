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
		Description: "Clients, projects, time tracking, and invoices for creative and consulting agencies. Czech-flavoured (IČO, DIČ, VAT modes). Apply to an empty workspace.",
		Install:     installAgency,
	})
}

func installAgency(ctx context.Context, tenantID, pgSchema string, ents *entities.Service, reps *reports.Service) error {
	specs := []entities.EntitySpec{
		clientsSpec(),
		suppliersSpec(),
		projectsSpec(),
		tasksSpec(),
		invoicesSpec(),
		invoiceItemsSpec(),
		timeEntriesSpec(),
	}
	for _, spec := range specs {
		if _, err := ents.Create(ctx, tenantID, pgSchema, &spec); err != nil {
			return fmt.Errorf("create %s: %w", spec.Name, err)
		}
	}
	if _, err := reps.Create(ctx, tenantID, agencyOverviewDashboard()); err != nil {
		return fmt.Errorf("create agency overview dashboard: %w", err)
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
		Description: "Klientské projekty a retainery.",
		Fields: []entities.FieldSpec{
			{Name: "name", DisplayName: "Název", DataType: entities.TypeText, IsRequired: true},
			{Name: "client", DisplayName: "Klient", DataType: entities.TypeReference, ReferenceEntity: "clients", IsRequired: true},
			{Name: "status", DisplayName: "Stav", DataType: entities.TypeText},
			{Name: "budget_type", DisplayName: "Typ rozpočtu", DataType: entities.TypeText},
			{Name: "budget", DisplayName: "Rozpočet", DataType: entities.TypeNumeric},
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
		Description: "Vystavené faktury pro klienty.",
		Fields: []entities.FieldSpec{
			{Name: "number", DisplayName: "Číslo", DataType: entities.TypeText, IsRequired: true, IsUnique: true},
			{Name: "client", DisplayName: "Klient", DataType: entities.TypeReference, ReferenceEntity: "clients", IsRequired: true},
			{Name: "issue_date", DisplayName: "Datum vystavení", DataType: entities.TypeDate, IsRequired: true},
			{Name: "due_date", DisplayName: "Splatnost", DataType: entities.TypeDate},
			{Name: "payment_date", DisplayName: "Zaplaceno dne", DataType: entities.TypeDate},
			{Name: "status", DisplayName: "Stav", DataType: entities.TypeText},
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

func agencyOverviewDashboard() reports.CreateDashboardInput {
	czk := map[string]interface{}{"number_format": "currency", "currency_code": "CZK"}
	intFmt := map[string]interface{}{"number_format": "integer"}
	decFmt := map[string]interface{}{"number_format": "decimal"}

	asc := &reports.Sort{Field: "label", Dir: "asc"}
	desc := &reports.Sort{Field: "value", Dir: "desc"}

	return reports.CreateDashboardInput{
		Name:        "Přehled agentury",
		Slug:        "agency_overview",
		Description: "Příjmy, hodiny a otevřené faktury na jednom místě.",
		Reports: []reports.CreateReportInput{
			{
				Title:      "Příjmy tento měsíc",
				Subtitle:   "Zaplacené faktury",
				WidgetType: reports.WidgetKPI,
				Width:      3,
				QuerySpec: reports.QuerySpec{
					Entity:          "invoices",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "total"},
					Filters:         []reports.Filter{{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"paid"`)}},
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
					Entity:          "invoices",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "total"},
					GroupBy:         &reports.GroupBy{Field: "issue_date", Bucket: reports.BucketMonth},
					Filters:         []reports.Filter{{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"paid"`)}},
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
					Entity:          "invoices",
					Aggregate:       &reports.Aggregate{Fn: reports.AggSum, Field: "total"},
					GroupBy:         &reports.GroupBy{Field: "client"},
					Filters:         []reports.Filter{{Field: "status", Op: reports.OpEq, Value: json.RawMessage(`"paid"`)}},
					DateFilterField: "issue_date",
					Sort:            desc,
					Limit:           10,
				},
				Options: czk,
			},
		},
	}
}
