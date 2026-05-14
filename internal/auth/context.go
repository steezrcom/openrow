package auth

import (
	"context"
	"net/http"
)

type ctxKey int

const (
	userCtx ctxKey = iota
	sessionCtx
	membershipCtx
)

type Identity struct {
	User       *User
	Session    *Session
	Membership *Membership
}

// FromContext returns the authenticated user and session if present.
// Only populated for requests that went through RequireAuth.
func FromContext(ctx context.Context) (*User, *Session, bool) {
	u, _ := ctx.Value(userCtx).(*User)
	s, _ := ctx.Value(sessionCtx).(*Session)
	if u == nil || s == nil {
		return nil, nil, false
	}
	return u, s, true
}

// MembershipFromContext returns the active membership if one was resolved.
func MembershipFromContext(ctx context.Context) (*Membership, bool) {
	m, ok := ctx.Value(membershipCtx).(*Membership)
	return m, ok && m != nil
}

func withIdentity(ctx context.Context, u *User, s *Session, m *Membership) context.Context {
	ctx = context.WithValue(ctx, userCtx, u)
	ctx = context.WithValue(ctx, sessionCtx, s)
	if m != nil {
		ctx = context.WithValue(ctx, membershipCtx, m)
	}
	return ctx
}

// Middleware wraps an http.Handler so that authenticated requests have the
// current user, session, and active membership (if any) attached to the context.
// Anonymous requests are allowed to pass through — use RequireAuth to block them.
type Middleware struct {
	Sessions    *SessionService
	Users       *UserService
	Memberships *MembershipService
}

func (m *Middleware) Attach(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		sess, err := m.Sessions.Lookup(r.Context(), cookie.Value)
		if err != nil || sess == nil {
			if err != nil {
				// log via default slog
			}
			next.ServeHTTP(w, r)
			return
		}
		user, err := m.Users.ByID(r.Context(), sess.UserID)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		var membership *Membership
		if sess.ActiveTenantID != nil {
			mb, mErr := m.Memberships.Get(r.Context(), user.ID, *sess.ActiveTenantID)
			if mErr == nil {
				membership = mb
			}
		}
		next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), user, sess, membership)))
	})
}

// RequireAuth blocks unauthenticated requests with 401 JSON.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := FromContext(r.Context()); !ok {
			writeJSONError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireMembership blocks requests without an active-org membership with 403 JSON.
func RequireMembership(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := FromContext(r.Context()); !ok {
			writeJSONError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if _, ok := MembershipFromContext(r.Context()); !ok {
			writeJSONError(w, http.StatusForbidden, "no active organization")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// roleRank gives an ordering for role comparisons.
// owner > admin > member; unknown roles compare as -1 (lower than any valid role).
func roleRank(r Role) int {
	switch r {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleMember:
		return 1
	}
	return -1
}

// RequireRole gates access to endpoints by minimum membership role.
// Owner is required for the most destructive operations (rotate webhook
// tokens, install templates, replace connector secrets, delete dashboards
// and entities). Admin is the default for write-class settings. Member
// covers ordinary row CRUD and reads. Use the lowest role that still
// keeps non-trusted seats from breaking the workspace.
func RequireRole(min Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m, ok := MembershipFromContext(r.Context())
		if !ok {
			writeJSONError(w, http.StatusForbidden, "no active organization")
			return
		}
		if roleRank(m.Role) < roleRank(min) {
			writeJSONError(w, http.StatusForbidden, "insufficient role")
			return
		}
		next.ServeHTTP(w, r)
	})
}
