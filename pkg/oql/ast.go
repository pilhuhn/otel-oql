package oql

// Query represents a complete OQL query
type Query struct {
	Signal     SignalType
	Operations []Operation
}

// SignalType represents the type of telemetry signal
type SignalType string

const (
	SignalMetrics SignalType = "metrics"
	SignalLogs    SignalType = "logs"
	SignalSpans   SignalType = "spans"
	SignalTraces  SignalType = "traces"
)

// Operation represents a query operation
type Operation interface {
	operation()
}

// WhereOp represents a where filter operation
type WhereOp struct {
	Condition Condition
}

func (WhereOp) operation() {}

// ExpandOp represents an expand operation
type ExpandOp struct {
	Type string // "trace"
}

func (ExpandOp) operation() {}

// CorrelateOp represents a correlate operation
type CorrelateOp struct {
	Signals []SignalType
}

func (CorrelateOp) operation() {}

// GetExemplarsOp represents a get_exemplars operation
type GetExemplarsOp struct{}

func (GetExemplarsOp) operation() {}

// SwitchContextOp represents a switch_context operation
type SwitchContextOp struct {
	Signal SignalType
}

func (SwitchContextOp) operation() {}

// ExtractOp represents an extract operation
type ExtractOp struct {
	Field string
	Alias string
}

func (ExtractOp) operation() {}

// FilterOp represents a filter operation (refines existing results)
type FilterOp struct {
	Condition Condition
}

func (FilterOp) operation() {}

// LimitOp represents a limit operation
type LimitOp struct {
	Count int
}

func (LimitOp) operation() {}

// AggregateOp represents an aggregation operation
type AggregateOp struct {
	Function string   // avg, min, max, count, sum
	Field    string   // field to aggregate (empty for count)
	Alias    string   // optional alias for result
}

func (AggregateOp) operation() {}

// GroupByOp represents a group by operation
type GroupByOp struct {
	Fields []string
}

func (GroupByOp) operation() {}

// SinceOp represents a time range filter (since X)
type SinceOp struct {
	Duration string // e.g., "1h", "30m", "2024-03-20"
}

func (SinceOp) operation() {}

// BetweenOp represents a time range filter (between X and Y)
type BetweenOp struct {
	Start string
	End   string
}

func (BetweenOp) operation() {}

// Condition represents a filter condition
type Condition interface {
	condition()
}

// BinaryCondition represents a binary comparison
type BinaryCondition struct {
	Left     string      // field name
	Operator string      // ==, !=, >, <, >=, <=
	Right    interface{} // value (string, int, duration, etc)
}

func (BinaryCondition) condition() {}

// AndCondition represents AND of multiple conditions
type AndCondition struct {
	Conditions []Condition
}

func (AndCondition) condition() {}

// OrCondition represents OR of multiple conditions
type OrCondition struct {
	Conditions []Condition
}

func (OrCondition) condition() {}
