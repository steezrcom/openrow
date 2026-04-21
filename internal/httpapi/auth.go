package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/steezrcom/steezr-erp/internal/auth"
)

type signupReq struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	// OrgName is optional; if set, an initial org is created and activated.
	OrgName string `json:"org_name,omitempty"`
	OrgSlug string `json:"org_slug,omitempty"`
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	var req signupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := s.users.Signup(r.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sess, err := s.sessions.Create(r.Context(), user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create session: "+err.Error())
		return
	}

	var activeOrgID *string
	if strings.TrimSpace(req.OrgName) != "" && strings.TrimSpace(req.OrgSlug) != "" {
		t, err := s.tenants.Create(r.Context(), strings.TrimSpace(req.OrgSlug), strings.TrimSpace(req.OrgName))
		if err == nil {
			_ = s.memberships.Add(r.Context(), user.ID, t.ID, auth.RoleOwner)
			_ = s.sessions.SetActiveTenant(r.Context(), sess.ID, t.ID)
			activeOrgID = &t.ID
		} else {
			s.log.Warn("initial org creation failed", "err", err)
		}
	}

	s.setSessionCookie(w, sess.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":          userDTO(user),
		"active_org_id": activeOrgID,
	})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := s.users.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	sess, err := s.sessions.Create(r.Context(), user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create session")
		return
	}

	// Default active org = first membership, if any.
	mships, _ := s.memberships.ForUser(r.Context(), user.ID)
	if len(mships) > 0 {
		_ = s.sessions.SetActiveTenant(r.Context(), sess.ID, mships[0].TenantID)
	}

	s.setSessionCookie(w, sess.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": userDTO(user),
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookie); err == nil && cookie.Value != "" {
		_ = s.sessions.Delete(r.Context(), cookie.Value)
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, sess, _ := auth.FromContext(r.Context())
	mships, err := s.memberships.ForUser(r.Context(), user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	active, _ := auth.MembershipFromContext(r.Context())
	_ = sess
	writeJSON(w, http.StatusOK, map[string]any{
		"user":              userDTO(user),
		"memberships":       membershipsDTO(mships),
		"active_membership": membershipDTOOrNil(active),
	})
}

type activateReq struct{}

func (s *Server) activateMembership(w http.ResponseWriter, r *http.Request) {
	user, sess, _ := auth.FromContext(r.Context())
	membershipID := r.PathValue("id")

	mships, err := s.memberships.ForUser(r.Context(), user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var target *auth.Membership
	for i := range mships {
		if mships[i].ID == membershipID {
			target = &mships[i]
			break
		}
	}
	if target == nil {
		writeErr(w, http.StatusNotFound, "membership not found")
		return
	}
	if err := s.sessions.SetActiveTenant(r.Context(), sess.ID, target.TenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active_membership": membershipDTO(target),
	})
}

type createOrgReq struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) createOrg(w http.ResponseWriter, r *http.Request) {
	user, sess, _ := auth.FromContext(r.Context())
	var req createOrgReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t, err := s.tenants.Create(r.Context(), strings.TrimSpace(req.Slug), strings.TrimSpace(req.Name))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.memberships.Add(r.Context(), user.ID, t.ID, auth.RoleOwner); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.sessions.SetActiveTenant(r.Context(), sess.ID, t.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	mb, err := s.memberships.Get(r.Context(), user.ID, t.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"membership": membershipDTO(mb),
	})
}
