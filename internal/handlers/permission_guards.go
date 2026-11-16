package handlers

import (
	"strings"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// permKey builds the "resource:action" key used by requireAnyPermission.
func permKey(resource, action string) string {
	return resource + ":" + action
}

// requireAnyPermission is requireAuth for screens shared between resources:
// it passes when the user holds at least one of the given "resource:action"
// keys in the request's organization. It is used where a page owned by one
// resource also serves a read-only picker elsewhere (for example the chat
// template picker for agents who hold chat:read but not templates:read).
// On failure it writes a 401/403 envelope and returns errEnvelopeSent.
func (a *App) requireAnyPermission(r *fastglue.Request, keys ...string) (orgID, userID uuid.UUID, err error) {
	orgID, userID, err = a.getOrgAndUserID(r)
	if err != nil {
		_ = r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		return uuid.Nil, uuid.Nil, errEnvelopeSent
	}
	for _, key := range keys {
		i := strings.LastIndex(key, ":")
		if i <= 0 {
			continue
		}
		if a.HasPermission(userID, key[:i], key[i+1:], orgID) {
			return orgID, userID, nil
		}
	}
	_ = r.SendErrorEnvelope(fasthttp.StatusForbidden, "Insufficient permissions", nil, "")
	return uuid.Nil, uuid.Nil, errEnvelopeSent
}
