package reports

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// WidgetType is the rendered shape of a report.
type WidgetType string

const (
	WidgetKPI   WidgetType = "kpi"
	WidgetBar   WidgetType = "bar"
	WidgetLine  WidgetType = "line"
	WidgetArea  WidgetType = "area"
	WidgetPie   WidgetType = "pie"
	WidgetTable WidgetType = "table"
)

func (w WidgetType) Valid() bool {
	switch w {
	case WidgetKPI, WidgetBar, WidgetLine, WidgetArea, WidgetPie, WidgetTable:
		return true
	}
	return false
}

// Aggregate reduces many rows to one value.
type AggregateFn string

const (
	AggCount AggregateFn = "count"
	AggSum   AggregateFn = "sum"
	AggAvg   AggregateFn = "avg"
	AggMin   AggregateFn = "min"
	AggMax   AggregateFn = "max"
)

func (a AggregateFn) Valid() bool {
	switch a {
	case AggCount, AggSum, AggAvg, AggMin, AggMax:
		return true
	}
	return false
}

// FilterOp enumerates supported comparisons.
type FilterOp string

const (
	OpEq         FilterOp = "eq"
	OpNe         FilterOp = "ne"
	OpGt         FilterOp = "gt"
	OpGte        FilterOp = "gte"
	OpLt         FilterOp = "lt"
	OpLte        FilterOp = "lte"
	OpContains   FilterOp = "contains"
	OpIn         FilterOp = "in"
	OpIsNull     FilterOp = "is_null"
	OpIsNotNull  FilterOp = "is_not_null"
)

// Bucket granularity for time-series group_by.
type Bucket string

const (
	BucketNone    Bucket = ""
	BucketDay     Bucket = "day"
	BucketWeek    Bucket = "week"
	BucketMonth   Bucket = "month"
	BucketQuarter Bucket = "quarter"
	BucketYear    Bucket = "year"
)

func (b Bucket) Valid() bool {
	switch b {
	case BucketNone, BucketDay, BucketWeek, BucketMonth, BucketQuarter, BucketYear:
		return true
	}
	return false
}

// Filter is one where-clause predicate. For OpIn, Value must be a JSON array.
type Filter struct {
	Field string          `json:"field"`
	Op    FilterOp        `json:"op"`
	Value json.RawMessage `json:"value,omitempty"`
}

// Aggregate is the reducer applied to each group (or the whole set if no group).
type Aggregate struct {
	Fn    AggregateFn `json:"fn"`
	Field string      `json:"field,omitempty"` // required for non-count aggregates
}

// GroupBy buckets rows before aggregation.
type GroupBy struct {
	Field  string `json:"field"`
	Bucket Bucket `json:"bucket,omitempty"` // only for date/timestamp fields
}

// Sort orders results. Field "bucket" or "value" refer to the aggregated columns.
type Sort struct {
	Field string `json:"field"`
	Dir   string `json:"dir"` // asc|desc
}

// QuerySpec describes one visualization's data request.
type QuerySpec struct {
	Entity    string     `json:"entity"`
	Filters   []Filter   `json:"filters,omitempty"`
	GroupBy   *GroupBy   `json:"group_by,omitempty"`
	SeriesBy  *GroupBy   `json:"series_by,omitempty"`
	Aggregate *Aggregate `json:"aggregate,omitempty"`
	Sort      *Sort      `json:"sort,omitempty"`
	Limit     int        `json:"limit,omitempty"`

	// DateFilterField names the date/timestamp field that reacts to the
	// dashboard's date range selector. Empty means the report ignores the
	// dashboard range (useful for KPIs that should always look at all data).
	DateFilterField string `json:"date_filter_field,omitempty"`

	// ComparePeriod asks KPI reports to render a delta vs an earlier window.
	// Only applied when DateFilterField is set and a range is supplied.
	// "previous_period": immediately preceding window of the same length.
	// "previous_year":   same window shifted back 1 year.
	ComparePeriod string `json:"compare_period,omitempty"`
}

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Validate performs lightweight checks that don't require entity metadata.
// Strong validation (field types, existence) happens in the executor.
func (s *QuerySpec) Validate() error {
	if !identRe.MatchString(s.Entity) {
		return fmt.Errorf("invalid entity name %q", s.Entity)
	}
	for i, f := range s.Filters {
		if !identRe.MatchString(f.Field) {
			return fmt.Errorf("filters[%d]: invalid field %q", i, f.Field)
		}
		switch f.Op {
		case OpEq, OpNe, OpGt, OpGte, OpLt, OpLte, OpContains, OpIn, OpIsNull, OpIsNotNull:
		default:
			return fmt.Errorf("filters[%d]: invalid op %q", i, f.Op)
		}
	}
	if s.GroupBy != nil {
		if !identRe.MatchString(s.GroupBy.Field) {
			return fmt.Errorf("group_by: invalid field %q", s.GroupBy.Field)
		}
		if !s.GroupBy.Bucket.Valid() {
			return fmt.Errorf("group_by: invalid bucket %q", s.GroupBy.Bucket)
		}
	}
	if s.Aggregate != nil {
		if !s.Aggregate.Fn.Valid() {
			return fmt.Errorf("aggregate: invalid fn %q", s.Aggregate.Fn)
		}
		if s.Aggregate.Fn != AggCount && !identRe.MatchString(s.Aggregate.Field) {
			return fmt.Errorf("aggregate: field required for %s", s.Aggregate.Fn)
		}
	}
	if s.Sort != nil {
		if s.Sort.Dir != "asc" && s.Sort.Dir != "desc" {
			return fmt.Errorf("sort: dir must be asc or desc")
		}
	}
	if s.DateFilterField != "" && !identRe.MatchString(s.DateFilterField) {
		return fmt.Errorf("invalid date_filter_field %q", s.DateFilterField)
	}
	if s.SeriesBy != nil {
		if !identRe.MatchString(s.SeriesBy.Field) {
			return fmt.Errorf("series_by: invalid field %q", s.SeriesBy.Field)
		}
		if !s.SeriesBy.Bucket.Valid() {
			return fmt.Errorf("series_by: invalid bucket %q", s.SeriesBy.Bucket)
		}
		if s.GroupBy == nil || s.Aggregate == nil {
			return fmt.Errorf("series_by requires both group_by and aggregate")
		}
	}
	switch s.ComparePeriod {
	case "", "previous_period", "previous_year":
	default:
		return fmt.Errorf("compare_period must be previous_period or previous_year")
	}
	return nil
}
