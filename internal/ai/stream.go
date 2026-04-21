package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/llm"
)

// StreamEvent is what RunStream emits to the caller. The client-side Chat
// panel appends text deltas incrementally and renders each tool start/end as
// a pill, so slow local models feel responsive.
type StreamEvent struct {
	Type    string   `json:"type"` // text_delta | tool_start | tool_end | done | error
	Delta   string   `json:"delta,omitempty"`
	Tool    *Action  `json:"tool,omitempty"`
	Text    string   `json:"text,omitempty"`
	Actions []Action `json:"actions,omitempty"`
	Message string   `json:"message,omitempty"`
}

// RunStream is the streaming analogue of Run. It emits text deltas and
// tool-call events as they happen, with a final "done" event carrying the
// full assistant text and the ordered actions list.
func (a *Agent) RunStream(
	ctx context.Context,
	tenantID, pgSchema string,
	history []ChatTurn,
	userMessage string,
	emit func(StreamEvent),
) error {
	if a == nil {
		return errors.New("agent not available")
	}
	cfg, err := a.llm.Resolve(ctx, tenantID)
	if err != nil {
		return err
	}
	client := llm.NewClient(cfg)

	existing, err := a.entities.List(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list entities: %w", err)
	}
	tools := a.buildTools(ctx, tenantID, pgSchema)
	msgs := buildMessageHistory(history, userMessage, existing)

	var actions []Action
	var finalText bytes.Buffer
	const maxIterations = 8

	for iter := 0; iter < maxIterations; iter++ {
		stream, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
			Model:      cfg.Model,
			Messages:   msgs,
			Tools:      tools.toolParams(),
			ToolChoice: "auto",
			MaxTokens:  2048,
			Stream:     true,
		})
		if err != nil {
			return fmt.Errorf("llm: %w", err)
		}

		// Accumulate deltas for this streamed response.
		accum := map[int]*toolCallAcc{}
		var assistantText bytes.Buffer

		for {
			chunk, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			if recvErr != nil {
				stream.Close()
				return fmt.Errorf("stream recv: %w", recvErr)
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			if choice.Delta.Content != "" {
				assistantText.WriteString(choice.Delta.Content)
				finalText.WriteString(choice.Delta.Content)
				emit(StreamEvent{Type: "text_delta", Delta: choice.Delta.Content})
			}
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				acc, ok := accum[idx]
				if !ok {
					acc = &toolCallAcc{Args: new(bytes.Buffer)}
					accum[idx] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Function.Name != "" {
					acc.Name = tc.Function.Name
				}
				acc.Args.WriteString(tc.Function.Arguments)
			}
		}
		stream.Close()

		if len(accum) == 0 {
			emit(StreamEvent{Type: "done", Text: finalText.String(), Actions: actions})
			return nil
		}

		// Assemble deterministic order (by index) for the assistant message +
		// tool execution.
		indices := make([]int, 0, len(accum))
		for i := range accum {
			indices = append(indices, i)
		}
		sort.Ints(indices)

		tcs := make([]openai.ToolCall, 0, len(accum))
		for _, i := range indices {
			acc := accum[i]
			tcs = append(tcs, openai.ToolCall{
				ID:   acc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      acc.Name,
					Arguments: acc.Args.String(),
				},
			})
		}
		// Append the assistant message so tool-role replies can reference
		// each ToolCallID.
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   assistantText.String(),
			ToolCalls: tcs,
		})

		for _, tc := range tcs {
			input := json.RawMessage(tc.Function.Arguments)
			emit(StreamEvent{
				Type: "tool_start",
				Tool: &Action{Tool: tc.Function.Name, Input: input},
			})

			exec := tools.run(ctx, tc.Function.Name, input)
			act := Action{
				Tool:       tc.Function.Name,
				Input:      input,
				Summary:    exec.Summary,
				EntityName: exec.EntityName,
				Error:      exec.ErrMsg(),
			}
			actions = append(actions, act)
			emit(StreamEvent{Type: "tool_end", Tool: &act})

			msgs = append(msgs, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Content:    exec.ResultText(),
			})
		}
	}

	return fmt.Errorf("agent exceeded %d tool iterations", maxIterations)
}

type toolCallAcc struct {
	ID   string
	Name string
	Args *bytes.Buffer
}
