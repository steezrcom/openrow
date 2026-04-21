package flows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openrow/openrow/internal/secrets"
)

type Service struct {
	pool *pgxpool.Pool
	enc  *secrets.Encrypter
}

func NewService(pool *pgxpool.Pool, enc *secrets.Encrypter) *Service {
	return &Service{pool: pool, enc: enc}
}

var (
	ErrNotFound         = errors.New("flow not found")
	ErrApprovalNotFound = errors.New("approval not found")
	ErrInvalidMode      = errors.New("invalid mode")
	ErrInvalidTrigger   = errors.New("invalid trigger kind")
)

type CreateFlowInput struct {
	Name          string
	Description   string
	Goal          string
	TriggerKind   TriggerKind
	TriggerConfig json.RawMessage
	ToolAllowlist []string
	Mode          Mode
	CreatedByUser string

	// WebhookSigningSecret, when non-empty, is encrypted and stored on
	// the flow so the webhook endpoint can verify incoming payloads
	// against the connector declared in trigger_config.webhook_connector_id.
	// Only meaningful for TriggerKind=webhook.
	WebhookSigningSecret string
}

func validateMode(m Mode) error {
	switch m {
	case ModeDryRun, ModeApprove, ModeAuto:
		return nil
	}
	return ErrInvalidMode
}

func validateTrigger(t TriggerKind) error {
	switch t {
	case TriggerManual, TriggerEntityEvent, TriggerWebhook, TriggerCron:
		return nil
	}
	return ErrInvalidTrigger
}

// CreateResult bundles the newly-created flow with any one-time secrets
// that must be shown to the caller right away (e.g. webhook token).
type CreateResult struct {
	Flow              *Flow
	WebhookTokenOnce  string // only set when TriggerKind == webhook; plaintext is unrecoverable after this call
}

func (s *Service) Create(ctx context.Context, tenantID string, in CreateFlowInput) (*CreateResult, error) {
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if in.Goal == "" {
		return nil, errors.New("goal is required")
	}
	if err := validateMode(in.Mode); err != nil {
		return nil, err
	}
	if err := validateTrigger(in.TriggerKind); err != nil {
		return nil, err
	}
	allowlist, err := json.Marshal(in.ToolAllowlist)
	if err != nil {
		return nil, err
	}
	triggerCfg := in.TriggerConfig
	if len(triggerCfg) == 0 {
		triggerCfg = json.RawMessage("{}")
	}
	var createdBy *string
	if in.CreatedByUser != "" {
		createdBy = &in.CreatedByUser
	}

	var (
		tokenPlaintext string
		tokenHash      []byte
		signingSecret  []byte
	)
	if in.TriggerKind == TriggerWebhook {
		tokenPlaintext, tokenHash, err = NewWebhookToken()
		if err != nil {
			return nil, err
		}
		if in.WebhookSigningSecret != "" {
			if s.enc == nil {
				return nil, errors.New("flows service missing encrypter; cannot store signing secret")
			}
			ct, encErr := s.enc.Encrypt([]byte(in.WebhookSigningSecret))
			if encErr != nil {
				return nil, fmt.Errorf("encrypt signing secret: %w", encErr)
			}
			signingSecret = ct
		}
	}

	// For cron flows, compute the first scheduled time from the cron
	// expression in trigger_config.cron. Reject malformed expressions
	// up front so the user sees the error immediately.
	var nextRunAt *time.Time
	if in.TriggerKind == TriggerCron {
		expr := cronExpression(triggerCfg)
		if expr == "" {
			return nil, errors.New("cron trigger requires trigger_config.cron")
		}
		schedule, perr := ParseCron(expr)
		if perr != nil {
			return nil, fmt.Errorf("invalid cron expression: %w", perr)
		}
		next := schedule.Next(time.Now()).UTC()
		nextRunAt = &next
	}

	var f Flow
	var triggerCfgOut, allowlistOut []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO openrow.flows
			(tenant_id, name, description, goal, trigger_kind, trigger_config,
			 tool_allowlist, mode, enabled, created_by_user_id,
			 webhook_token_hash, webhook_secret, next_run_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, true, $9, $10, $11, $12)
		RETURNING id, tenant_id, name, COALESCE(description, ''), goal, trigger_kind,
		          trigger_config, tool_allowlist, mode, enabled, created_by_user_id,
		          created_at, updated_at`,
		tenantID, in.Name, in.Description, in.Goal,
		string(in.TriggerKind), triggerCfg, allowlist, string(in.Mode), createdBy,
		tokenHash, signingSecret, nextRunAt,
	).Scan(&f.ID, &f.TenantID, &f.Name, &f.Description, &f.Goal, &f.TriggerKind,
		&triggerCfgOut, &allowlistOut, &f.Mode, &f.Enabled, &f.CreatedByUserID,
		&f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	f.TriggerConfig = triggerCfgOut
	if err := json.Unmarshal(allowlistOut, &f.ToolAllowlist); err != nil {
		return nil, err
	}
	return &CreateResult{Flow: &f, WebhookTokenOnce: tokenPlaintext}, nil
}

func (s *Service) List(ctx context.Context, tenantID string) ([]Flow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description, ''), goal, trigger_kind,
		       trigger_config, tool_allowlist, mode, enabled, created_by_user_id,
		       created_at, updated_at
		FROM openrow.flows
		WHERE tenant_id = $1
		ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Flow, 0)
	for rows.Next() {
		var f Flow
		var triggerCfg, allowlist []byte
		if err := rows.Scan(&f.ID, &f.TenantID, &f.Name, &f.Description, &f.Goal, &f.TriggerKind,
			&triggerCfg, &allowlist, &f.Mode, &f.Enabled, &f.CreatedByUserID,
			&f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.TriggerConfig = triggerCfg
		if err := json.Unmarshal(allowlist, &f.ToolAllowlist); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (*Flow, error) {
	var f Flow
	var triggerCfg, allowlist []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description, ''), goal, trigger_kind,
		       trigger_config, tool_allowlist, mode, enabled, created_by_user_id,
		       created_at, updated_at
		FROM openrow.flows
		WHERE tenant_id = $1 AND id = $2`, tenantID, id).
		Scan(&f.ID, &f.TenantID, &f.Name, &f.Description, &f.Goal, &f.TriggerKind,
			&triggerCfg, &allowlist, &f.Mode, &f.Enabled, &f.CreatedByUserID,
			&f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f.TriggerConfig = triggerCfg
	if err := json.Unmarshal(allowlist, &f.ToolAllowlist); err != nil {
		return nil, err
	}
	return &f, nil
}

type UpdateFlowInput struct {
	Name          *string
	Description   *string
	Goal          *string
	TriggerKind   *TriggerKind
	TriggerConfig *json.RawMessage
	ToolAllowlist *[]string
	Mode          *Mode
	Enabled       *bool
}

func (s *Service) Update(ctx context.Context, tenantID, id string, in UpdateFlowInput) (*Flow, error) {
	existing, err := s.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		existing.Name = *in.Name
	}
	if in.Description != nil {
		existing.Description = *in.Description
	}
	if in.Goal != nil {
		existing.Goal = *in.Goal
	}
	if in.TriggerKind != nil {
		if err := validateTrigger(*in.TriggerKind); err != nil {
			return nil, err
		}
		existing.TriggerKind = *in.TriggerKind
	}
	if in.TriggerConfig != nil {
		existing.TriggerConfig = *in.TriggerConfig
	}
	if in.ToolAllowlist != nil {
		existing.ToolAllowlist = *in.ToolAllowlist
	}
	if in.Mode != nil {
		if err := validateMode(*in.Mode); err != nil {
			return nil, err
		}
		existing.Mode = *in.Mode
	}
	if in.Enabled != nil {
		existing.Enabled = *in.Enabled
	}

	allowlist, err := json.Marshal(existing.ToolAllowlist)
	if err != nil {
		return nil, err
	}
	triggerCfg := existing.TriggerConfig
	if len(triggerCfg) == 0 {
		triggerCfg = json.RawMessage("{}")
	}

	// Recompute next_run_at when a cron flow's expression (or trigger_kind)
	// has changed. For non-cron flows we clear the column so the scheduler
	// ignores them.
	var nextRunAt *time.Time
	if existing.TriggerKind == TriggerCron {
		expr := cronExpression(triggerCfg)
		if expr == "" {
			return nil, errors.New("cron trigger requires trigger_config.cron")
		}
		schedule, perr := ParseCron(expr)
		if perr != nil {
			return nil, fmt.Errorf("invalid cron expression: %w", perr)
		}
		next := schedule.Next(time.Now()).UTC()
		nextRunAt = &next
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE openrow.flows
		SET name = $1, description = NULLIF($2, ''), goal = $3,
		    trigger_kind = $4, trigger_config = $5, tool_allowlist = $6,
		    mode = $7, enabled = $8, next_run_at = $9, updated_at = now()
		WHERE tenant_id = $10 AND id = $11`,
		existing.Name, existing.Description, existing.Goal,
		string(existing.TriggerKind), triggerCfg, allowlist,
		string(existing.Mode), existing.Enabled, nextRunAt,
		tenantID, id)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, id)
}

func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM openrow.flows WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListEntityEventMatches returns enabled flows whose trigger is
// entity_event on the given (tenant, entity, event). Event kind is matched
// against trigger_config.events (jsonb array); if the array is missing,
// "insert" is the default match — the overwhelmingly common case.
func (s *Service) ListEntityEventMatches(ctx context.Context, tenantID, entity, eventKind string) ([]Flow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description, ''), goal, trigger_kind,
		       trigger_config, tool_allowlist, mode, enabled, created_by_user_id,
		       created_at, updated_at
		FROM openrow.flows
		WHERE tenant_id = $1
		  AND enabled = true
		  AND trigger_kind = 'entity_event'
		  AND trigger_config->>'entity' = $2
		  AND (
		    (NOT (trigger_config ? 'events') AND $3 = 'insert')
		    OR trigger_config->'events' @> to_jsonb($3::text)
		  )`, tenantID, entity, eventKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Flow, 0)
	for rows.Next() {
		var f Flow
		var triggerCfg, allowlist []byte
		if err := rows.Scan(&f.ID, &f.TenantID, &f.Name, &f.Description, &f.Goal, &f.TriggerKind,
			&triggerCfg, &allowlist, &f.Mode, &f.Enabled, &f.CreatedByUserID,
			&f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.TriggerConfig = triggerCfg
		if err := json.Unmarshal(allowlist, &f.ToolAllowlist); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// dueCronFlows returns enabled cron-triggered flows whose next_run_at has
// passed. Used by the scheduler; not exposed through the HTTP API.
func (s *Service) dueCronFlows(ctx context.Context) ([]Flow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description, ''), goal, trigger_kind,
		       trigger_config, tool_allowlist, mode, enabled, created_by_user_id,
		       created_at, updated_at
		FROM openrow.flows
		WHERE enabled = true
		  AND trigger_kind = 'cron'
		  AND next_run_at IS NOT NULL
		  AND next_run_at <= now()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Flow, 0)
	for rows.Next() {
		var f Flow
		var triggerCfg, allowlist []byte
		if err := rows.Scan(&f.ID, &f.TenantID, &f.Name, &f.Description, &f.Goal, &f.TriggerKind,
			&triggerCfg, &allowlist, &f.Mode, &f.Enabled, &f.CreatedByUserID,
			&f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.TriggerConfig = triggerCfg
		if err := json.Unmarshal(allowlist, &f.ToolAllowlist); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// setNextRun updates a flow's scheduled next run time. Called by the
// scheduler after dispatching (to advance) and by Create/Update when a
// cron flow is (re)defined.
func (s *Service) setNextRun(ctx context.Context, flowID string, at time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE openrow.flows SET next_run_at = $1 WHERE id = $2`,
		at.UTC(), flowID)
	return err
}

// --- Runs ----------------------------------------------------------------

func (s *Service) CreateRun(ctx context.Context, flow *Flow, triggerPayload json.RawMessage) (*Run, error) {
	if len(triggerPayload) == 0 {
		triggerPayload = json.RawMessage("{}")
	}
	var r Run
	err := s.pool.QueryRow(ctx, `
		INSERT INTO openrow.flow_runs
			(flow_id, tenant_id, trigger_payload, status, mode)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, flow_id, tenant_id, trigger_payload, status, mode,
		          message_history, COALESCE(error, ''), started_at, finished_at`,
		flow.ID, flow.TenantID, triggerPayload, string(StatusQueued), string(flow.Mode),
	).Scan(&r.ID, &r.FlowID, &r.TenantID, &r.TriggerPayload, &r.Status, &r.Mode,
		&r.MessageHistory, &r.Error, &r.StartedAt, &r.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) GetRun(ctx context.Context, tenantID, runID string) (*Run, error) {
	var r Run
	err := s.pool.QueryRow(ctx, `
		SELECT id, flow_id, tenant_id, trigger_payload, status, mode,
		       message_history, COALESCE(error, ''), started_at, finished_at
		FROM openrow.flow_runs
		WHERE tenant_id = $1 AND id = $2`, tenantID, runID).
		Scan(&r.ID, &r.FlowID, &r.TenantID, &r.TriggerPayload, &r.Status, &r.Mode,
			&r.MessageHistory, &r.Error, &r.StartedAt, &r.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

func (s *Service) ListRuns(ctx context.Context, tenantID, flowID string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, flow_id, tenant_id, trigger_payload, status, mode,
		       '[]'::jsonb AS message_history, COALESCE(error, ''), started_at, finished_at
		FROM openrow.flow_runs
		WHERE tenant_id = $1 AND flow_id = $2
		ORDER BY started_at DESC
		LIMIT $3`, tenantID, flowID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Run, 0)
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.FlowID, &r.TenantID, &r.TriggerPayload, &r.Status, &r.Mode,
			&r.MessageHistory, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateRunProgress snapshots message_history and status in one write. The
// runner calls this at every checkpoint so an abrupt process death leaves
// behind a resumable run rather than a silently-lost one.
func (s *Service) UpdateRunProgress(ctx context.Context, runID string, status RunStatus, messageHistory json.RawMessage, errMsg string) error {
	var finishedAt *time.Time
	if status == StatusSucceeded || status == StatusFailed {
		now := time.Now().UTC()
		finishedAt = &now
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE openrow.flow_runs
		SET status = $1, message_history = $2, error = NULLIF($3, ''), finished_at = COALESCE($4, finished_at)
		WHERE id = $5`,
		string(status), messageHistory, errMsg, finishedAt, runID)
	return err
}

// --- Steps ---------------------------------------------------------------

func (s *Service) AppendStep(ctx context.Context, runID string, kind StepKind, content any) error {
	raw, err := json.Marshal(content)
	if err != nil {
		return err
	}
	// position = max(position) + 1 atomically via subquery.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO openrow.flow_run_steps (flow_run_id, position, kind, content)
		SELECT $1,
		       COALESCE((SELECT max(position)+1 FROM openrow.flow_run_steps WHERE flow_run_id = $1), 0),
		       $2, $3`,
		runID, string(kind), raw)
	return err
}

func (s *Service) ListSteps(ctx context.Context, runID string) ([]Step, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, flow_run_id, position, kind, content, created_at
		FROM openrow.flow_run_steps
		WHERE flow_run_id = $1
		ORDER BY position`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Step, 0)
	for rows.Next() {
		var s Step
		if err := rows.Scan(&s.ID, &s.RunID, &s.Position, &s.Kind, &s.Content, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// --- Approvals -----------------------------------------------------------

func (s *Service) CreateApproval(ctx context.Context, tenantID, runID, toolCallID, toolName string, toolInput json.RawMessage) (*Approval, error) {
	var a Approval
	err := s.pool.QueryRow(ctx, `
		INSERT INTO openrow.flow_approvals
			(flow_run_id, tenant_id, tool_call_id, tool_name, tool_input, status)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING id, flow_run_id, tenant_id, tool_call_id, tool_name, tool_input,
		          status, COALESCE(rejection_reason, ''), requested_at, resolved_at, resolved_by_user_id`,
		runID, tenantID, toolCallID, toolName, toolInput,
	).Scan(&a.ID, &a.RunID, &a.TenantID, &a.ToolCallID, &a.ToolName, &a.ToolInput,
		&a.Status, &a.RejectionReason, &a.RequestedAt, &a.ResolvedAt, &a.ResolvedByUserID)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Service) ListPendingApprovals(ctx context.Context, tenantID string) ([]Approval, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, flow_run_id, tenant_id, tool_call_id, tool_name, tool_input,
		       status, COALESCE(rejection_reason, ''), requested_at, resolved_at, resolved_by_user_id
		FROM openrow.flow_approvals
		WHERE tenant_id = $1 AND status = 'pending'
		ORDER BY requested_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Approval, 0)
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.RunID, &a.TenantID, &a.ToolCallID, &a.ToolName, &a.ToolInput,
			&a.Status, &a.RejectionReason, &a.RequestedAt, &a.ResolvedAt, &a.ResolvedByUserID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Service) GetApproval(ctx context.Context, tenantID, id string) (*Approval, error) {
	var a Approval
	err := s.pool.QueryRow(ctx, `
		SELECT id, flow_run_id, tenant_id, tool_call_id, tool_name, tool_input,
		       status, COALESCE(rejection_reason, ''), requested_at, resolved_at, resolved_by_user_id
		FROM openrow.flow_approvals
		WHERE tenant_id = $1 AND id = $2`, tenantID, id).
		Scan(&a.ID, &a.RunID, &a.TenantID, &a.ToolCallID, &a.ToolName, &a.ToolInput,
			&a.Status, &a.RejectionReason, &a.RequestedAt, &a.ResolvedAt, &a.ResolvedByUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	return &a, err
}

// ResolveApproval flips an approval's status + records who decided.
// Returns the fully-scoped approval so the caller can drive a resume.
func (s *Service) ResolveApproval(ctx context.Context, tenantID, id, userID string, approved bool, rejectionReason string) (*Approval, error) {
	status := ApprovalApproved
	if !approved {
		status = ApprovalRejected
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE openrow.flow_approvals
		SET status = $1,
		    rejection_reason = NULLIF($2, ''),
		    resolved_at = now(),
		    resolved_by_user_id = $3
		WHERE tenant_id = $4 AND id = $5 AND status = 'pending'`,
		string(status), rejectionReason, userID, tenantID, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("approval not pending")
	}
	return s.GetApproval(ctx, tenantID, id)
}
