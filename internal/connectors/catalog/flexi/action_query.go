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
// (tenant, connector). The field_map keys are canonical roles, values are
// vendor column names. Implementations live elsewhere — discovery.Repo and a
// service shim that pulls the tenant ID from context.
type MappingResolver interface {
	Resolve(ctx context.Context, tenantID, connectorID, role string) (evidence string, fieldMap map[string]string, err error)
}

var resolver MappingResolver

// SetResolver wires the production MappingResolver. Called once from main()
// to avoid an import cycle between this package and the discovery repo.
func SetResolver(mr MappingResolver) { resolver = mr }

func queryHandler(ctx context.Context, creds map[string]string, in json.RawMessage) (any, error) {
	if resolver == nil {
		return nil, errors.New("discovery resolver not initialised")
	}
	return queryWithResolver(ctx, creds, in, resolver, false)
}

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

	evidence, fieldMap, err := mr.Resolve(ctx, connectors.TenantFromContext(ctx), ConnectorID, in.Role)
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

	reverse := make(map[string]string, len(fieldMap))
	for canonical, vendor := range fieldMap {
		reverse[vendor] = canonical
	}
	projection := make(map[string]bool, len(in.Fields))
	for _, f := range in.Fields {
		projection[f] = true
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		mapped := map[string]any{}
		for k, v := range r {
			alias, ok := reverse[k]
			switch {
			case ok && (len(projection) == 0 || projection[alias]):
				mapped[alias] = v
			case !ok && len(projection) == 0:
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
