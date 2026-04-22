// Package linear integrates with Linear's GraphQL API for issue
// creation and state transitions. Auth is a personal API key; unlike
// most APIs the header is the raw key, without a "Bearer " prefix.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/connectors"
)

const endpoint = "https://api.linear.app/graphql"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "linear",
		Name:        "Linear",
		Description: "Create and update Linear issues.",
		Category:    "dev",
		Homepage:    "https://linear.app",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "api_key", Label: "API key", Kind: connectors.FieldSecret, Required: true,
				Placeholder: "lin_api_…",
				Help:        "Linear → Settings → API → Personal API keys."},
			{Name: "default_team_id", Label: "Default team ID", Kind: connectors.FieldText, Required: false,
				Help: "Used when create_issue doesn't specify a team."},
		},
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "create_issue",
			Name:        "Create issue",
			Description: "Create an issue in a team. Returns the new issue's id, identifier (e.g. \"ENG-42\") and URL.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id":     map[string]any{"type": "string", "description": "Linear team UUID; falls back to default_team_id."},
					"title":       map[string]any{"type": "string"},
					"description": map[string]any{"type": "string", "description": "Markdown body."},
					"assignee_id": map[string]any{"type": "string"},
					"priority":    map[string]any{"type": "integer", "description": "0=none, 1=urgent, 2=high, 3=normal, 4=low."},
					"state_id":    map[string]any{"type": "string"},
					"label_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"title"},
			},
			Handler: createIssue,
		},
		{
			ID:          "update_issue_state",
			Name:        "Update issue state",
			Description: "Move an issue to a different workflow state.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issue_id": map[string]any{"type": "string"},
					"state_id": map[string]any{"type": "string"},
				},
				"required": []string{"issue_id", "state_id"},
			},
			Handler: updateIssueState,
		},
	}
}

type createIn struct {
	TeamID      string   `json:"team_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	AssigneeID  string   `json:"assignee_id"`
	Priority    *int     `json:"priority"`
	StateID     string   `json:"state_id"`
	LabelIDs    []string `json:"label_ids"`
}

const createMutation = `mutation($input: IssueCreateInput!){
  issueCreate(input: $input){
    success
    issue { id identifier url title }
  }
}`

func createIssue(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in createIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("title is required")
	}
	teamID := in.TeamID
	if teamID == "" {
		teamID = creds["default_team_id"]
	}
	if teamID == "" {
		return nil, errors.New("team_id is required (no default_team_id configured)")
	}
	input := map[string]any{"teamId": teamID, "title": in.Title}
	if in.Description != "" {
		input["description"] = in.Description
	}
	if in.AssigneeID != "" {
		input["assigneeId"] = in.AssigneeID
	}
	if in.Priority != nil {
		input["priority"] = *in.Priority
	}
	if in.StateID != "" {
		input["stateId"] = in.StateID
	}
	if len(in.LabelIDs) > 0 {
		input["labelIds"] = in.LabelIDs
	}
	var resp struct {
		IssueCreate struct {
			Success bool `json:"success"`
			Issue   struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				URL        string `json:"url"`
				Title      string `json:"title"`
			} `json:"issue"`
		} `json:"issueCreate"`
	}
	if err := gqlCall(ctx, creds, createMutation, map[string]any{"input": input}, &resp); err != nil {
		return nil, err
	}
	if !resp.IssueCreate.Success {
		return nil, errors.New("linear: issueCreate returned success=false")
	}
	return resp.IssueCreate.Issue, nil
}

type updateStateIn struct {
	IssueID string `json:"issue_id"`
	StateID string `json:"state_id"`
}

const updateStateMutation = `mutation($id: String!, $input: IssueUpdateInput!){
  issueUpdate(id: $id, input: $input){
    success
    issue { id identifier state { id name } }
  }
}`

func updateIssueState(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in updateStateIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if in.IssueID == "" || in.StateID == "" {
		return nil, errors.New("issue_id and state_id are required")
	}
	var resp struct {
		IssueUpdate struct {
			Success bool `json:"success"`
			Issue   struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				State      struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"state"`
			} `json:"issue"`
		} `json:"issueUpdate"`
	}
	vars := map[string]any{"id": in.IssueID, "input": map[string]any{"stateId": in.StateID}}
	if err := gqlCall(ctx, creds, updateStateMutation, vars, &resp); err != nil {
		return nil, err
	}
	if !resp.IssueUpdate.Success {
		return nil, errors.New("linear: issueUpdate returned success=false")
	}
	return resp.IssueUpdate.Issue, nil
}

const viewerQuery = `{ viewer { id name email } }`

func test(ctx context.Context, creds map[string]string) error {
	var resp struct {
		Viewer struct {
			ID string `json:"id"`
		} `json:"viewer"`
	}
	if err := gqlCall(ctx, creds, viewerQuery, nil, &resp); err != nil {
		return err
	}
	if resp.Viewer.ID == "" {
		return errors.New("linear: viewer query returned empty id")
	}
	return nil
}

func gqlCall(ctx context.Context, creds map[string]string, query string, variables map[string]any, out any) error {
	key := strings.TrimSpace(creds["api_key"])
	if key == "" {
		return errors.New("linear: api_key missing")
	}
	payload := map[string]any{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("linear: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("linear: status %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("linear: decode: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("linear: %s", envelope.Errors[0].Message)
	}
	if out != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("linear: decode data: %w", err)
		}
	}
	return nil
}
