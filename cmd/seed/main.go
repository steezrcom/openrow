// cmd/seed is a dev fixture tool. It creates (or reuses) a user and a tenant,
// installs the agency template, and seeds realistic demo rows.
//
// Usage:
//
//	make seed                # defaults: demo@openrow.local / openrow123 / slug=demo
//	make seed RESET=1        # drop the tenant first, then re-seed
//	go run ./cmd/seed -help
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/reports"
	"github.com/openrow/openrow/internal/store"
	"github.com/openrow/openrow/internal/templates"
	"github.com/openrow/openrow/internal/tenant"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	email := flag.String("email", "demo@openrow.local", "demo user's email")
	password := flag.String("password", "openrow123", "demo user's password (min 10 chars)")
	userName := flag.String("name", "Demo User", "demo user's display name")
	tenantSlug := flag.String("tenant-slug", "demo", "tenant slug (a-z0-9_, max 31 chars)")
	tenantName := flag.String("tenant-name", "Demo Agency", "tenant display name")
	reset := flag.Bool("reset", false, "drop the tenant schema + metadata before re-seeding")
	forceData := flag.Bool("force-data", false, "re-insert demo rows even if entities already contain data")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPool(ctx, dbURL)
	if err != nil {
		log.Error("connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	if err := run(ctx, log, pool, opts{
		email:      *email,
		password:   *password,
		userName:   *userName,
		tenantSlug: *tenantSlug,
		tenantName: *tenantName,
		reset:      *reset,
		forceData:  *forceData,
	}); err != nil {
		log.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

type opts struct {
	email, password, userName string
	tenantSlug, tenantName    string
	reset, forceData          bool
}

func run(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, o opts) error {
	users := auth.NewUserService(pool)
	memberships := auth.NewMembershipService(pool)
	tenants := tenant.NewService(pool)
	entSvc := entities.NewService(pool)
	dashSvc := reports.NewService(pool)

	if o.reset {
		if err := dropTenant(ctx, pool, o.tenantSlug); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		log.Info("tenant dropped", "slug", o.tenantSlug)
	}

	user, err := ensureUser(ctx, pool, users, o.email, o.userName, o.password)
	if err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}
	log.Info("user ready", "email", user.Email, "id", user.ID)

	tn, err := ensureTenant(ctx, tenants, o.tenantSlug, o.tenantName)
	if err != nil {
		return fmt.Errorf("ensure tenant: %w", err)
	}
	log.Info("tenant ready", "slug", tn.Slug, "schema", tn.PGSchema)

	if err := memberships.Add(ctx, user.ID, tn.ID, auth.RoleOwner); err != nil {
		return fmt.Errorf("add membership: %w", err)
	}

	if _, err := entSvc.Get(ctx, tn.ID, "clients"); err != nil {
		tpl, ok := templates.Get("agency")
		if !ok {
			return errors.New("agency template not registered")
		}
		if err := tpl.Install(ctx, tn.ID, tn.PGSchema, entSvc, dashSvc); err != nil {
			return fmt.Errorf("install agency template: %w", err)
		}
		log.Info("agency template installed")
	} else {
		log.Info("agency template already present, skipping install")
	}

	clientsEnt, err := entSvc.Get(ctx, tn.ID, "clients")
	if err != nil {
		return err
	}
	n, err := entSvc.CountRows(ctx, tn.PGSchema, clientsEnt)
	if err != nil {
		return err
	}
	if n > 0 && !o.forceData {
		log.Info("demo rows already present, skipping data seed", "clients", n)
		log.Info("done", "email", user.Email, "password", o.password, "tenant", tn.Slug)
		return nil
	}

	if err := seedDemoData(ctx, entSvc, tn.ID, tn.PGSchema, log); err != nil {
		return fmt.Errorf("seed data: %w", err)
	}

	log.Info("done", "email", user.Email, "password", o.password, "tenant", tn.Slug)
	return nil
}

func ensureUser(ctx context.Context, pool *pgxpool.Pool, users *auth.UserService, email, name, password string) (*auth.User, error) {
	u, err := users.Signup(ctx, email, name, password)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, auth.ErrUserExists) {
		return nil, err
	}
	var id string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM openrow.users WHERE email = $1`, email,
	).Scan(&id); err != nil {
		return nil, err
	}
	return users.ByID(ctx, id)
}

func ensureTenant(ctx context.Context, tenants *tenant.Service, slug, name string) (*tenant.Tenant, error) {
	t, err := tenants.BySlug(ctx, slug)
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, tenant.ErrNotFound) {
		return nil, err
	}
	return tenants.Create(ctx, slug, name)
}

func dropTenant(ctx context.Context, pool *pgxpool.Pool, slug string) error {
	t, err := (&tenantLookup{pool: pool}).bySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if _, err := pool.Exec(ctx,
		fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, pgx.Identifier{t.schema}.Sanitize()),
	); err != nil {
		return fmt.Errorf("drop schema: %w", err)
	}
	// fields.reference_entity_id is a self-ref FK without ON DELETE CASCADE,
	// so we can't rely on tenants→entities cascade to clean up.
	if _, err := pool.Exec(ctx,
		`DELETE FROM openrow.fields WHERE entity_id IN (SELECT id FROM openrow.entities WHERE tenant_id = $1)`,
		t.id,
	); err != nil {
		return fmt.Errorf("delete fields: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM openrow.entities WHERE tenant_id = $1`, t.id); err != nil {
		return fmt.Errorf("delete entities: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM openrow.tenants WHERE id = $1`, t.id); err != nil {
		return fmt.Errorf("delete tenant row: %w", err)
	}
	return nil
}

type tenantLookup struct {
	pool *pgxpool.Pool
}

func (t *tenantLookup) bySlug(ctx context.Context, slug string) (struct{ id, schema string }, error) {
	var out struct{ id, schema string }
	err := t.pool.QueryRow(ctx,
		`SELECT id, pg_schema FROM openrow.tenants WHERE slug = $1`, slug,
	).Scan(&out.id, &out.schema)
	return out, err
}

// --- demo data ---------------------------------------------------------------

func seedDemoData(ctx context.Context, svc *entities.Service, tenantID, schema string, log *slog.Logger) error {
	rng := rand.New(rand.NewSource(42))
	today := time.Now().UTC().Truncate(24 * time.Hour)

	clientIDs, err := insertClients(ctx, svc, tenantID, schema)
	if err != nil {
		return fmt.Errorf("clients: %w", err)
	}
	log.Info("seeded clients", "n", len(clientIDs))

	supplierIDs, err := insertSuppliers(ctx, svc, tenantID, schema)
	if err != nil {
		return fmt.Errorf("suppliers: %w", err)
	}
	log.Info("seeded suppliers", "n", len(supplierIDs))

	projectIDs, err := insertProjects(ctx, svc, tenantID, schema, clientIDs, today)
	if err != nil {
		return fmt.Errorf("projects: %w", err)
	}
	log.Info("seeded projects", "n", len(projectIDs))

	tasksByProject, err := insertTasks(ctx, svc, tenantID, schema, projectIDs, today, rng)
	if err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	taskTotal := 0
	for _, ts := range tasksByProject {
		taskTotal += len(ts)
	}
	log.Info("seeded tasks", "n", taskTotal)

	invoiceIDs, err := insertInvoices(ctx, svc, tenantID, schema, clientIDs, today, rng)
	if err != nil {
		return fmt.Errorf("invoices: %w", err)
	}
	log.Info("seeded invoices", "n", len(invoiceIDs))

	itemCount, err := insertInvoiceItems(ctx, svc, tenantID, schema, invoiceIDs, projectIDs, rng)
	if err != nil {
		return fmt.Errorf("invoice items: %w", err)
	}
	log.Info("seeded invoice items", "n", itemCount)

	teCount, err := insertTimeEntries(ctx, svc, tenantID, schema, projectIDs, tasksByProject, today, rng)
	if err != nil {
		return fmt.Errorf("time entries: %w", err)
	}
	log.Info("seeded time entries", "n", teCount)

	return nil
}

type clientRow struct {
	name, ico, dic, email, phone, billing, vat, currency string
	terms                                                int
}

func insertClients(ctx context.Context, svc *entities.Service, tenantID, schema string) ([]string, error) {
	ent, err := svc.Get(ctx, tenantID, "clients")
	if err != nil {
		return nil, err
	}
	rows := []clientRow{
		{"Studio Mazanec s.r.o.", "12345678", "CZ12345678", "info@mazanec.cz", "+420 777 111 222", "Vinohradská 10, 12000 Praha 2", "standard", "CZK", 14},
		{"Kavárna Dobré Ráno", "87654321", "", "kavarna@dobrerano.cz", "+420 777 222 333", "Masarykovo nám. 3, 60200 Brno", "non_payer", "CZK", 7},
		{"BioMarket CZ s.r.o.", "23456789", "CZ23456789", "ucetni@biomarket.cz", "+420 777 333 444", "Dlouhá 21, 11000 Praha 1", "standard", "CZK", 14},
		{"Architekti Veselý & Partneři", "34567890", "CZ34567890", "office@vesely-architects.cz", "+420 777 444 555", "Vlnená 12, 60200 Brno", "standard", "CZK", 30},
		{"Pekárna U Kostela", "45678901", "", "pekarna@ukostela.cz", "+420 777 555 666", "Náměstí 4, 38001 Český Krumlov", "non_payer", "CZK", 7},
		{"TechnoData Solutions s.r.o.", "56789012", "CZ56789012", "billing@technodata.cz", "+420 777 666 777", "Sokolská 55, 12000 Praha 2", "reverse_charge", "CZK", 30},
		{"Nakladatelství Luna", "67890123", "CZ67890123", "info@luna-books.cz", "+420 777 777 888", "Karlovo nám. 9, 12000 Praha 2", "standard", "CZK", 14},
		{"Bistro Zelené Lístky", "78901234", "", "bistro@zelenelistky.cz", "+420 777 888 999", "Žižkova 30, 37001 České Budějovice", "non_payer", "CZK", 7},
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		id, err := svc.InsertRow(ctx, schema, ent, map[string]string{
			"name":               r.name,
			"ico":                r.ico,
			"dic":                r.dic,
			"email":              r.email,
			"phone":              r.phone,
			"billing_address":    r.billing,
			"vat_mode":           r.vat,
			"currency":           r.currency,
			"payment_terms_days": fmt.Sprintf("%d", r.terms),
		})
		if err != nil {
			return nil, fmt.Errorf("insert client %q: %w", r.name, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

type supplierRow struct {
	name, email, phone, ico, subjectType, status string
	hourlyRate                                   float64
}

func insertSuppliers(ctx context.Context, svc *entities.Service, tenantID, schema string) ([]string, error) {
	ent, err := svc.Get(ctx, tenantID, "suppliers")
	if err != nil {
		return nil, err
	}
	rows := []supplierRow{
		{"Anna Novotná", "anna@freelance.cz", "+420 608 111 222", "11112222", "OSVČ", "active", 850},
		{"Petr Svoboda", "petr@svoboda.cz", "+420 608 222 333", "22223333", "OSVČ", "active", 1200},
		{"Jana Dvořáková", "jana.d@studio.cz", "+420 608 333 444", "33334444", "s.r.o.", "active", 1500},
		{"Martin Černý", "martin@devshop.cz", "+420 608 444 555", "44445555", "s.r.o.", "active", 1800},
		{"Tomáš Kovařík", "tomas@fotograf.cz", "+420 608 555 666", "55556666", "OSVČ", "inactive", 900},
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		id, err := svc.InsertRow(ctx, schema, ent, map[string]string{
			"name":         r.name,
			"email":        r.email,
			"phone":        r.phone,
			"ico":          r.ico,
			"subject_type": r.subjectType,
			"status":       r.status,
			"hourly_rate":  fmt.Sprintf("%.2f", r.hourlyRate),
			"currency":     "CZK",
		})
		if err != nil {
			return nil, fmt.Errorf("insert supplier %q: %w", r.name, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

type projectRow struct {
	name, status, budgetType string
	clientIdx                int
	budget, hourlyRate       float64
	billable                 bool
	startOffsetDays          int
	durationDays             int
}

func insertProjects(ctx context.Context, svc *entities.Service, tenantID, schema string, clientIDs []string, today time.Time) ([]string, error) {
	ent, err := svc.Get(ctx, tenantID, "projects")
	if err != nil {
		return nil, err
	}
	rows := []projectRow{
		{"Rebranding 2026", "active", "fixed", 0, 180000, 1400, true, -90, 150},
		{"Web redesign", "active", "hourly", 0, 0, 1400, true, -30, 120},
		{"Kampaň jarní menu", "completed", "fixed", 1, 35000, 1000, true, -120, 45},
		{"Sociální sítě (retainer)", "active", "retainer", 1, 12000, 0, true, -180, 365},
		{"E-shop migrace", "active", "fixed", 2, 240000, 1600, true, -60, 180},
		{"Obsahový marketing", "active", "retainer", 2, 18000, 0, true, -90, 365},
		{"Katalog 2026", "active", "fixed", 3, 95000, 1500, true, -45, 90},
		{"Interiérový branding", "paused", "fixed", 3, 60000, 1500, true, -150, 120},
		{"Foto produktů", "completed", "fixed", 4, 25000, 900, true, -60, 21},
		{"Dev sprint Q1", "active", "hourly", 5, 0, 1800, true, -30, 90},
		{"Newsletter systém", "completed", "fixed", 6, 45000, 1300, true, -100, 30},
		{"Menu tisk", "active", "fixed", 7, 12000, 800, true, -20, 30},
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		input := map[string]string{
			"name":        r.name,
			"client":      clientIDs[r.clientIdx],
			"status":      r.status,
			"budget_type": r.budgetType,
			"billable":    boolStr(r.billable),
			"start_date":  today.AddDate(0, 0, r.startOffsetDays).Format("2006-01-02"),
			"end_date":    today.AddDate(0, 0, r.startOffsetDays+r.durationDays).Format("2006-01-02"),
		}
		if r.budget > 0 {
			input["budget"] = fmt.Sprintf("%.2f", r.budget)
		}
		if r.hourlyRate > 0 {
			input["hourly_rate"] = fmt.Sprintf("%.2f", r.hourlyRate)
		}
		id, err := svc.InsertRow(ctx, schema, ent, input)
		if err != nil {
			return nil, fmt.Errorf("insert project %q: %w", r.name, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func insertTasks(ctx context.Context, svc *entities.Service, tenantID, schema string, projectIDs []string, today time.Time, rng *rand.Rand) (map[string][]string, error) {
	ent, err := svc.Get(ctx, tenantID, "tasks")
	if err != nil {
		return nil, err
	}
	names := []string{
		"Kick-off a briefing", "Návrh konceptu", "Design review", "Copywriting",
		"Produkce grafiky", "Korektura", "Implementace", "QA a testing",
		"Publikace", "Reporting",
	}
	assignees := []string{"Johnny", "Petra", "Tomáš", "Klára"}
	statuses := []string{"todo", "in_progress", "done", "done", "done"}

	byProject := make(map[string][]string, len(projectIDs))
	for i, pid := range projectIDs {
		count := 2 + rng.Intn(3)
		for j := 0; j < count; j++ {
			taskName := names[(i*3+j)%len(names)]
			due := today.AddDate(0, 0, -10+rng.Intn(60))
			id, err := svc.InsertRow(ctx, schema, ent, map[string]string{
				"name":            fmt.Sprintf("%s — %d", taskName, j+1),
				"project":         pid,
				"status":          statuses[rng.Intn(len(statuses))],
				"assignee":        assignees[rng.Intn(len(assignees))],
				"due_date":        due.Format("2006-01-02"),
				"estimated_hours": fmt.Sprintf("%.1f", 2+rng.Float64()*14),
				"billable":        boolStr(rng.Intn(10) > 1),
			})
			if err != nil {
				return nil, fmt.Errorf("insert task: %w", err)
			}
			byProject[pid] = append(byProject[pid], id)
		}
	}
	return byProject, nil
}

func insertInvoices(ctx context.Context, svc *entities.Service, tenantID, schema string, clientIDs []string, today time.Time, rng *rand.Rand) ([]string, error) {
	ent, err := svc.Get(ctx, tenantID, "invoices")
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, 36)
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)

	slot := 1
	for m := 0; m < 12; m++ {
		month := monthStart.AddDate(0, m, 0)
		count := 3 + rng.Intn(2)
		for k := 0; k < count; k++ {
			issue := month.AddDate(0, 0, 3+rng.Intn(22))
			clientIdx := rng.Intn(len(clientIDs))
			subtotal := 15000 + float64(rng.Intn(110000))
			vat := round2(subtotal * 0.21)
			total := round2(subtotal + vat)
			status, paymentDate := invoiceStatus(issue, today, rng)

			input := map[string]string{
				"number":     fmt.Sprintf("INV-%s-%03d", issue.Format("200601"), slot),
				"client":     clientIDs[clientIdx],
				"issue_date": issue.Format("2006-01-02"),
				"due_date":   issue.AddDate(0, 0, 14).Format("2006-01-02"),
				"status":     status,
				"currency":   "CZK",
				"subtotal":   fmt.Sprintf("%.2f", subtotal),
				"vat_amount": fmt.Sprintf("%.2f", vat),
				"total":      fmt.Sprintf("%.2f", total),
				"vat_mode":   "standard",
			}
			if !paymentDate.IsZero() {
				input["payment_date"] = paymentDate.Format("2006-01-02")
			}
			id, err := svc.InsertRow(ctx, schema, ent, input)
			if err != nil {
				return nil, fmt.Errorf("insert invoice: %w", err)
			}
			ids = append(ids, id)
			slot++
		}
	}
	return ids, nil
}

func invoiceStatus(issue, today time.Time, rng *rand.Rand) (string, time.Time) {
	age := int(today.Sub(issue).Hours() / 24)
	switch {
	case age > 45:
		return "paid", issue.AddDate(0, 0, 10+rng.Intn(8))
	case age > 14:
		if rng.Intn(7) == 0 {
			return "overdue", time.Time{}
		}
		return "paid", issue.AddDate(0, 0, 8+rng.Intn(10))
	default:
		return "pending", time.Time{}
	}
}

func insertInvoiceItems(ctx context.Context, svc *entities.Service, tenantID, schema string, invoiceIDs, projectIDs []string, rng *rand.Rand) (int, error) {
	ent, err := svc.Get(ctx, tenantID, "invoice_items")
	if err != nil {
		return 0, err
	}
	descriptions := []string{
		"Design práce", "Copywriting", "Produkce grafiky", "Hodiny seniorního konzultanta",
		"Retainer — měsíční správa", "Fotografická produkce", "Vývoj a implementace",
		"Správa kampaní", "Konzultace a strategie",
	}
	units := []string{"hod", "ks", "měs"}
	count := 0
	for _, invID := range invoiceIDs {
		items := 1 + rng.Intn(3)
		for i := 0; i < items; i++ {
			qty := 1 + rng.Intn(20)
			unit := units[rng.Intn(len(units))]
			unitPrice := 800 + float64(rng.Intn(1800))
			subtotal := float64(qty) * unitPrice
			input := map[string]string{
				"invoice":     invID,
				"description": descriptions[rng.Intn(len(descriptions))],
				"quantity":    fmt.Sprintf("%d", qty),
				"unit":        unit,
				"unit_price":  fmt.Sprintf("%.2f", unitPrice),
				"vat_rate":    "21",
				"subtotal":    fmt.Sprintf("%.2f", subtotal),
			}
			if rng.Intn(3) != 0 {
				input["project"] = projectIDs[rng.Intn(len(projectIDs))]
			}
			if _, err := svc.InsertRow(ctx, schema, ent, input); err != nil {
				return 0, fmt.Errorf("insert invoice item: %w", err)
			}
			count++
		}
	}
	return count, nil
}

func insertTimeEntries(ctx context.Context, svc *entities.Service, tenantID, schema string, projectIDs []string, tasksByProject map[string][]string, today time.Time, rng *rand.Rand) (int, error) {
	ent, err := svc.Get(ctx, tenantID, "time_entries")
	if err != nil {
		return 0, err
	}
	people := []string{"Johnny", "Petra", "Tomáš", "Klára"}
	descriptions := []string{
		"Design iterace", "Klientské meeting", "Copy draft", "Produkce",
		"Vývoj", "Code review", "QA", "Retro / plánování", "Onboarding",
	}
	count := 0
	for d := 0; d < 60; d++ {
		day := today.AddDate(0, 0, -d)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		perDay := 2 + rng.Intn(3)
		for i := 0; i < perDay; i++ {
			hours := 0.5 + float64(rng.Intn(15))*0.5
			pid := projectIDs[rng.Intn(len(projectIDs))]
			input := map[string]string{
				"project":     pid,
				"person":      people[rng.Intn(len(people))],
				"date":        day.Format("2006-01-02"),
				"hours":       fmt.Sprintf("%.1f", hours),
				"description": descriptions[rng.Intn(len(descriptions))],
				"billable":    boolStr(rng.Intn(10) > 1),
				"rate":        "1200",
			}
			tasks := tasksByProject[pid]
			if len(tasks) > 0 && rng.Intn(2) == 0 {
				input["task"] = tasks[rng.Intn(len(tasks))]
			}
			if _, err := svc.InsertRow(ctx, schema, ent, input); err != nil {
				return 0, fmt.Errorf("insert time entry: %w", err)
			}
			count++
		}
	}
	return count, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
