package handlers

import (
	"io"
	"regexp"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// stagingIDPattern constrains a staging id to characters that cannot escape the
// media directory. The id reaches us from the client and is used to build a
// filesystem path, so anything outside this set is refused rather than sanitized.
var stagingIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

// rsvpReminderStagingKey returns the pseudo campaign id under which a staged
// reminder media file is saved, or "" if the id is not safe to use in a path.
//
// It is deliberately a flat "staging-<id>" filename, not a subdirectory:
// saveCampaignMedia hardcodes subdir "campaigns" and ensureMediaDir creates only
// that, so "staging/<id>" would fail to write.
func rsvpReminderStagingKey(stagingID string) string {
	if !stagingIDPattern.MatchString(stagingID) {
		return ""
	}
	return "staging-" + stagingID
}

// UploadRSVPReminderMedia stages header media for a reminder send before the
// campaign exists. createRSVPReminderCampaign creates and enqueues its campaign
// in one call, but UploadCampaignMedia (campaigns.go:1591) needs an existing
// campaign id — so this saves the file under a pseudo campaign id
// (rsvpReminderStagingKey) that the send request references by staging_id.
//
// Mirrors UploadCampaignMedia's multipart handling, 16MB cap and MIME sniffing
// rather than inventing a second convention.
func (a *App) UploadRSVPReminderMedia(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	eventID, err := parsePathUUID(r, "id", "RSVP event")
	if err != nil {
		return nil
	}
	if _, err := findByIDAndOrg[models.RSVPEvent](a.DB, r, eventID, orgID, "RSVP event"); err != nil {
		return nil
	}

	form, err := r.RequestCtx.MultipartForm()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid multipart form", nil, "")
	}

	files := form.File["file"]
	if len(files) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "No file provided", nil, "")
	}

	fileHeader := files[0]
	file, err := fileHeader.Open()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to open file", nil, "")
	}
	defer func() { _ = file.Close() }()

	// Read file content (limit to 16MB)
	const maxMediaSize = 16 << 20 // 16MB
	data, err := io.ReadAll(io.LimitReader(file, maxMediaSize+1))
	if err != nil {
		a.Log.Error("Failed to read file", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to read file", nil, "")
	}
	if len(data) > maxMediaSize {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "File too large. Maximum size is 16MB", nil, "")
	}

	mimeType := detectCampaignMediaMimeType(fileHeader.Filename, fileHeader.Header.Get("Content-Type"), data)
	if !isAllowedCampaignMediaMimeType(mimeType) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Unsupported file type: "+mimeType, nil, "")
	}

	// uuid.New().String() contains only hex and "-", so it always satisfies
	// stagingIDPattern.
	stagingID := uuid.New().String()
	key := rsvpReminderStagingKey(stagingID)
	if key == "" {
		a.Log.Error("Generated staging id failed stagingIDPattern", "staging_id", stagingID)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to stage media", nil, "")
	}

	if _, err := a.saveCampaignMedia(key, data, mimeType); err != nil {
		a.Log.Error("Failed to save staged reminder media", "error", err, "staging_id", stagingID, "mime_type", mimeType, "filename", fileHeader.Filename)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save media locally", nil, "")
	}

	return r.SendEnvelope(map[string]interface{}{
		"staging_id": stagingID,
		"filename":   fileHeader.Filename,
		"mime_type":  mimeType,
	})
}
