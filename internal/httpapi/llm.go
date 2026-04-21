package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/llm"
)

func (s *Server) listLLMProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"providers": llm.Providers})
}

func (s *Server) getLLMConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	safe, err := s.llm.GetSafe(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": safe})
}

type putLLMConfigReq struct {
	Provider string  `json:"provider"`
	BaseURL  string  `json:"base_url"`
	APIKey   *string `json:"api_key,omitempty"`
	Model    string  `json:"model"`
}

func (s *Server) putLLMConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in putLLMConfigReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	safe, err := s.llm.Set(r.Context(), m.TenantID, llm.SetInput{
		Provider: in.Provider,
		BaseURL:  in.BaseURL,
		APIKey:   in.APIKey,
		Model:    in.Model,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": safe})
}

func (s *Server) deleteLLMConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	if err := s.llm.Delete(r.Context(), m.TenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listLLMModels calls the provider's /v1/models using the supplied base_url +
// api_key from the request body (not the saved config), so the settings UI can
// preview models before the user commits.
type listModelsReq struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

func (s *Server) listLLMModels(w http.ResponseWriter, r *http.Request) {
	var in listModelsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	models, err := llm.ListModels(r.Context(), in.BaseURL, in.APIKey)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
}

type testLLMReq struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// testLLM runs a three-stage probe (models, chat, tool call) against the
// supplied creds. Returns a structured result so the UI can show which stages
// passed: "chat works but tool calling doesn't" is a valid, actionable state.
func (s *Server) testLLM(w http.ResponseWriter, r *http.Request) {
	var in testLLMReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	result := llm.Test(r.Context(), in.BaseURL, in.APIKey, in.Model)
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}
