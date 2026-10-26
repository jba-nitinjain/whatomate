package handlers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const defaultLeadAPIVersion = "v21.0"

// leadAPIVersionRe restricts the Graph API version to the "vNN.N" form so it
// cannot be used to inject path segments into Graph API URLs.
var leadAPIVersionRe = regexp.MustCompile(`^v\d+\.\d+$`)

// leadWebhookURL returns the public URL Meta must call for leadgen events.
func (a *App) leadWebhookURL() string {
	base := a.Config.Server.PublicURL
	if base == "" {
		base = fmt.Sprintf("http://localhost:%d", a.Config.Server.Port)
	}
	return strings.TrimSuffix(base, "/") + "/api/webhook/leadgen"
}

// normalizeLeadAPIVersion applies the default and validates the format.
func normalizeLeadAPIVersion(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return defaultLeadAPIVersion, true
	}
	return v, leadAPIVersionRe.MatchString(v)
}

// loadLeadSource fetches an org-scoped lead source by the {id} path param.
// It sends the error envelope itself and returns nil on failure.
func (a *App) loadLeadSource(r *fastglue.Request, orgID uuid.UUID) *models.LeadSource {
	sourceID, err := parsePathUUID(r, "id", "lead source")
	if err != nil {
		return nil
	}
	source, err := findByIDAndOrg[models.LeadSource](a.DB, r, sourceID, orgID, "Lead source")
	if err != nil {
		return nil
	}
	return source
}

// leadAuth resolves the caller and checks a lead_ads permission. ok is false
// when an error envelope has already been sent.
func (a *App) leadAuth(r *fastglue.Request, action string) (orgID, userID uuid.UUID, ok bool) {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		_ = r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		return uuid.Nil, uuid.Nil, false
	}
	if err := a.requirePermission(r, userID, models.ResourceLeadAds, action); err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return orgID, userID, true
}

// isUniqueViolation reports whether err is a Postgres unique_violation (23505).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}
