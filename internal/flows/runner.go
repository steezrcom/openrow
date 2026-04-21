package flows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/ai"
	"github.com/openrow/openrow/internal/llm"
	"github.com/openrow/openrow/internal/tenant"
)

// Runner drives a single flow execution — its own agent loop with the
// caller-supplied toolset. Write-class tool calls are intercepted based on
// the run's Mode: dry-run short-circuits them, approve suspends the run,
// auto executes them.
type Runner struct {
	svc     *Service
	llm     *llm.Service
	agent   *ai.Agent
	tenants *tenant.Service
}

func NewRunner(svc *Service, llmSvc *llm.Service, agent *ai.Agent, tenants *tenant.Service) *Runner {
	return &Runner{svc: svc, llm: llmSvc, agent: agent, tenants: tenants}
}

const (
	runnerMaxIterations = 20
	runnerMaxDuration   = 60 * time.Second
	runnerSystemPrompt  = `You are an OpenRow automation. Follow the flow's goal, using the provided tools. Work autonomously: don't ask questions. When the goal is achieved or you can't proceed, stop making tool calls and summarize what you did in one short sentence. If a tool you need isn't available, stop and explain precisely which one is missing.`
)

// RunManual creates a new run for a manually-triggered flow and drives it
// to completion or suspension. Returns the final Run state.
func (r *Runner) RunManual(ctx context.Context, flow *Flow) (*Run, error) {
	return r.Start(ctx, flow, json.RawMessage(`{"kind":"manual"}`))
}

// Start creates a run with the given trigger payload and executes it.
func (r *Runner) Start(ctx context.Context, flow *Flow, triggerPayload json.RawMessage) (*Run, error) {
	run, err := r.svc.CreateRun(ctx, flow, triggerPayload)
	if err != nil {
		return nil, err
	}
	return r.drive(ctx, flow, run, nil)
}

// Resume continues a suspended run after its pending approval was resolved.
// If approved == false, the intercepted tool's result is the rejection reason.
func (r *Runner) Resume(ctx context.Context, flow *Flow, run *Run, approval *Approval) (*Run, error) {
	if run.Status != StatusAwaitingApproval {
		return run, fmt.Errorf("run %s is not awaiting approval (status=%s)", run.ID, run.Status)
	}
	// Synthesize the tool result that goes back to the LLM for the intercepted call.
	var toolResult string
	if approval.Status == ApprovalApproved {
		t, err := r.fetchTenantByID(ctx, run.TenantID)
		if err != nil {
			return nil, err
		}
		tools := r.agent.BuildToolset(ctx, run.TenantID, t.PGSchema)
		exec := tools.Invoke(ctx, approval.ToolName, approval.ToolInput)
		toolResult = exec.ResultText()
		_ = r.svc.AppendStep(ctx, run.ID, StepToolResult, stepToolResultPayload(approval.ToolCallID, approval.ToolName, toolResult, exec.Err))
	} else {
		reason := approval.RejectionReason
		if reason == "" {
			reason = "rejected by user"
		}
		toolResult = "(rejected) " + reason
		_ = r.svc.AppendStep(ctx, run.ID, StepToolResult, stepToolResultPayload(approval.ToolCallID, approval.ToolName, toolResult, nil))
	}

	// Append the tool result to the persisted message history and continue.
	msgs, err := decodeHistory(run.MessageHistory)
	if err != nil {
		return nil, err
	}
	msgs = append(msgs, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: approval.ToolCallID,
		Content:    toolResult,
	})

	return r.drive(ctx, flow, run, msgs)
}

// drive is the main loop. If resumeMsgs is non-nil, it's the message history
// to continue from (used on Resume). Otherwise we seed from system + goal +
// trigger payload.
func (r *Runner) drive(ctx context.Context, flow *Flow, run *Run, resumeMsgs []openai.ChatCompletionMessage) (*Run, error) {
	ctx, cancel := context.WithTimeout(ctx, runnerMaxDuration)
	defer cancel()

	cfg, err := r.llm.Resolve(ctx, run.TenantID)
	if err != nil {
		return r.fail(ctx, run, fmt.Errorf("llm: %w", err))
	}
	client := llm.NewClient(cfg)

	t, err := r.fetchTenantByID(ctx, run.TenantID)
	if err != nil {
		return r.fail(ctx, run, err)
	}
	fullToolset := r.agent.BuildToolset(ctx, run.TenantID, t.PGSchema)
	tools, err := filterToolset(fullToolset, flow.ToolAllowlist)
	if err != nil {
		return r.fail(ctx, run, err)
	}

	var msgs []openai.ChatCompletionMessage
	if resumeMsgs != nil {
		msgs = resumeMsgs
	} else {
		msgs = seedMessages(flow, run)
	}

	if err := r.markRunning(ctx, run); err != nil {
		return nil, err
	}

	for iter := 0; iter < runnerMaxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return r.fail(ctx, run, fmt.Errorf("timeout or cancel: %w", err))
		}

		resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:      cfg.Model,
			Messages:   msgs,
			Tools:      tools.ToolParams(),
			ToolChoice: "auto",
			MaxTokens:  2048,
		})
		if err != nil {
			return r.fail(ctx, run, fmt.Errorf("llm: %w", err))
		}
		if len(resp.Choices) == 0 {
			return r.fail(ctx, run, errors.New("llm returned no choices"))
		}
		choice := resp.Choices[0]

		// Agent produced a final assistant message → done.
		if len(choice.Message.ToolCalls) == 0 {
			_ = r.svc.AppendStep(ctx, run.ID, StepAgentMessage, map[string]any{
				"text": choice.Message.Content,
			})
			msgs = append(msgs, choice.Message)
			return r.finish(ctx, run, StatusSucceeded, msgs, "")
		}

		// Append assistant turn (including its tool calls).
		msgs = append(msgs, choice.Message)
		if choice.Message.Content != "" {
			_ = r.svc.AppendStep(ctx, run.ID, StepAgentMessage, map[string]any{
				"text": choice.Message.Content,
			})
		}

		for _, tc := range choice.Message.ToolCalls {
			toolInput := json.RawMessage(tc.Function.Arguments)
			_ = r.svc.AppendStep(ctx, run.ID, StepToolCall, map[string]any{
				"tool_call_id": tc.ID,
				"name":         tc.Function.Name,
				"input":        toolInput,
			})

			// Allowlist check.
			if !allowlistContains(flow.ToolAllowlist, tc.Function.Name) {
				errText := fmt.Sprintf("tool %q is not in this flow's allowlist", tc.Function.Name)
				_ = r.svc.AppendStep(ctx, run.ID, StepMutationBlocked, map[string]any{
					"tool_call_id": tc.ID,
					"name":         tc.Function.Name,
					"reason":       "not_allowed",
				})
				msgs = append(msgs, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: tc.ID,
					Content:    errText,
				})
				continue
			}

			toolDef, ok := tools.Get(tc.Function.Name)
			if !ok {
				errText := fmt.Sprintf("tool %q is unknown", tc.Function.Name)
				msgs = append(msgs, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: tc.ID,
					Content:    errText,
				})
				continue
			}

			// Mode check.
			if toolDef.Mutates {
				switch run.Mode {
				case ModeDryRun:
					synthetic := fmt.Sprintf("(dry-run) would have called %s with %s", tc.Function.Name, toolInput)
					_ = r.svc.AppendStep(ctx, run.ID, StepMutationBlocked, map[string]any{
						"tool_call_id": tc.ID,
						"name":         tc.Function.Name,
						"reason":       "dry_run",
						"synthetic":    synthetic,
					})
					msgs = append(msgs, openai.ChatCompletionMessage{
						Role:       openai.ChatMessageRoleTool,
						ToolCallID: tc.ID,
						Content:    synthetic,
					})
					continue
				case ModeApprove:
					if _, err := r.svc.CreateApproval(ctx, run.TenantID, run.ID, tc.ID, tc.Function.Name, toolInput); err != nil {
						return r.fail(ctx, run, fmt.Errorf("create approval: %w", err))
					}
					_ = r.svc.AppendStep(ctx, run.ID, StepApprovalRequested, map[string]any{
						"tool_call_id": tc.ID,
						"name":         tc.Function.Name,
					})
					// Persist message history up to and including the assistant
					// turn that requested this tool, then suspend.
					return r.suspend(ctx, run, msgs)
				}
			}

			// Execute.
			exec := tools.Invoke(ctx, tc.Function.Name, toolInput)
			result := exec.ResultText()
			_ = r.svc.AppendStep(ctx, run.ID, StepToolResult, stepToolResultPayload(tc.ID, tc.Function.Name, result, exec.Err))
			msgs = append(msgs, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}

	return r.fail(ctx, run, fmt.Errorf("agent exceeded %d iterations", runnerMaxIterations))
}

// --- helpers ------------------------------------------------------------

func (r *Runner) markRunning(ctx context.Context, run *Run) error {
	run.Status = StatusRunning
	return r.svc.UpdateRunProgress(ctx, run.ID, StatusRunning, run.MessageHistory, "")
}

func (r *Runner) finish(ctx context.Context, run *Run, status RunStatus, msgs []openai.ChatCompletionMessage, errMsg string) (*Run, error) {
	raw, err := encodeHistory(msgs)
	if err != nil {
		return nil, err
	}
	run.Status = status
	run.MessageHistory = raw
	run.Error = errMsg
	if err := r.svc.UpdateRunProgress(ctx, run.ID, status, raw, errMsg); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *Runner) fail(ctx context.Context, run *Run, err error) (*Run, error) {
	msg := err.Error()
	_ = r.svc.UpdateRunProgress(ctx, run.ID, StatusFailed, run.MessageHistory, msg)
	run.Status = StatusFailed
	run.Error = msg
	return run, err
}

func (r *Runner) suspend(ctx context.Context, run *Run, msgs []openai.ChatCompletionMessage) (*Run, error) {
	raw, err := encodeHistory(msgs)
	if err != nil {
		return nil, err
	}
	run.Status = StatusAwaitingApproval
	run.MessageHistory = raw
	if err := r.svc.UpdateRunProgress(ctx, run.ID, StatusAwaitingApproval, raw, ""); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *Runner) fetchTenantByID(ctx context.Context, tenantID string) (*tenant.Tenant, error) {
	return r.tenants.ByID(ctx, tenantID)
}

func seedMessages(flow *Flow, run *Run) []openai.ChatCompletionMessage {
	system := runnerSystemPrompt + "\n\nFlow goal: " + flow.Goal + "\nMode: " + string(flow.Mode) + "\nAllowed tools: " + strings.Join(flow.ToolAllowlist, ", ")
	return []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: system},
		{Role: openai.ChatMessageRoleUser, Content: "Trigger payload: " + string(run.TriggerPayload) + "\n\nBegin."},
	}
}

// filterToolset returns a toolset limited to the allowlist. Allowlist is
// treated as exact tool-name matches for now; wildcard + entity-scoped
// filters ("entity.update:invoices") come later.
func filterToolset(full *ai.Toolset, allowlist []string) (*ai.Toolset, error) {
	allow := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		allow[name] = struct{}{}
	}
	var kept []ai.Tool
	for _, t := range full.Tools() {
		if _, ok := allow[t.Name]; ok {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		return nil, errors.New("tool_allowlist doesn't match any registered tool")
	}
	return ai.NewToolset(kept), nil
}

func allowlistContains(allowlist []string, name string) bool {
	for _, n := range allowlist {
		if n == name {
			return true
		}
	}
	return false
}

func encodeHistory(msgs []openai.ChatCompletionMessage) (json.RawMessage, error) {
	b, err := json.Marshal(msgs)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func decodeHistory(raw json.RawMessage) ([]openai.ChatCompletionMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var msgs []openai.ChatCompletionMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func stepToolResultPayload(toolCallID, name, result string, err error) map[string]any {
	out := map[string]any{
		"tool_call_id": toolCallID,
		"name":         name,
		"result":       result,
	}
	if err != nil {
		out["error"] = err.Error()
	}
	return out
}
