// Package flows implements agent-authored automations: persistent flows
// with a goal, a trigger, a tool allowlist and a permission mode. Each run
// is a full agent loop whose write-class tool calls can be intercepted
// (dry-run, blocked-pending-approval, or executed).
package flows

import (
	"encoding/json"
	"time"
)

type Mode string

const (
	ModeDryRun  Mode = "dry_run" // every write is simulated + logged, not executed
	ModeApprove Mode = "approve" // every write pauses pending user approval
	ModeAuto    Mode = "auto"    // writes execute unattended
)

type TriggerKind string

const (
	TriggerManual      TriggerKind = "manual"
	TriggerEntityEvent TriggerKind = "entity_event"
	TriggerWebhook     TriggerKind = "webhook"
	TriggerCron        TriggerKind = "cron"
)

type RunStatus string

const (
	StatusQueued           RunStatus = "queued"
	StatusRunning          RunStatus = "running"
	StatusAwaitingApproval RunStatus = "awaiting_approval"
	StatusSucceeded        RunStatus = "succeeded"
	StatusFailed           RunStatus = "failed"
)

type StepKind string

const (
	StepAgentMessage      StepKind = "agent_message"
	StepToolCall          StepKind = "tool_call"
	StepToolResult        StepKind = "tool_result"
	StepMutationBlocked   StepKind = "mutation_blocked"
	StepApprovalRequested StepKind = "approval_requested"
)

type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
	ApprovalExpired  ApprovalStatus = "expired"
)

// Flow is the persisted definition.
type Flow struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenant_id"`
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	Goal             string          `json:"goal"`
	TriggerKind      TriggerKind     `json:"trigger_kind"`
	TriggerConfig    json.RawMessage `json:"trigger_config"`
	ToolAllowlist    []string        `json:"tool_allowlist"`
	Mode             Mode            `json:"mode"`
	Enabled          bool            `json:"enabled"`
	CreatedByUserID  *string         `json:"created_by_user_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// Run is one execution attempt. MessageHistory is the authoritative state
// the runner uses to resume after an approval; Steps is a humans-facing log.
type Run struct {
	ID              string          `json:"id"`
	FlowID          string          `json:"flow_id"`
	TenantID        string          `json:"tenant_id"`
	TriggerPayload  json.RawMessage `json:"trigger_payload"`
	Status          RunStatus       `json:"status"`
	Mode            Mode            `json:"mode"`
	MessageHistory  json.RawMessage `json:"-"`
	Error           string          `json:"error,omitempty"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
}

type Step struct {
	ID        string          `json:"id"`
	RunID     string          `json:"flow_run_id"`
	Position  int             `json:"position"`
	Kind      StepKind        `json:"kind"`
	Content   json.RawMessage `json:"content"`
	CreatedAt time.Time       `json:"created_at"`
}

type Approval struct {
	ID               string          `json:"id"`
	RunID            string          `json:"flow_run_id"`
	TenantID         string          `json:"tenant_id"`
	ToolCallID       string          `json:"tool_call_id"`
	ToolName         string          `json:"tool_name"`
	ToolInput        json.RawMessage `json:"tool_input"`
	Status           ApprovalStatus  `json:"status"`
	RejectionReason  string          `json:"rejection_reason,omitempty"`
	RequestedAt      time.Time       `json:"requested_at"`
	ResolvedAt       *time.Time      `json:"resolved_at,omitempty"`
	ResolvedByUserID *string         `json:"resolved_by_user_id,omitempty"`
}
