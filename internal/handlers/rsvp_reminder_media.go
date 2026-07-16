package handlers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// loadStagedRSVPReminderMedia reads back the bytes UploadRSVPReminderMedia
// staged under stagingID and derives their MIME type from the staged file
// itself, not from a client-supplied value. The send request previously
// carried a "staging_mime_type" field that was used to rebuild the staged
// file's extension for reading it; a mismatched echo of that field pointed
// HeaderMediaLocalPath at a file that does not exist. Locating the file by
// glob and re-detecting its type removes that trust boundary entirely.
func (a *App) loadStagedRSVPReminderMedia(stagingID string) (data []byte, mimeType string, err error) {
	key := rsvpReminderStagingKey(stagingID)
	if key == "" {
		return nil, "", fmt.Errorf("invalid media reference")
	}
	matches, err := filepath.Glob(filepath.Join(a.getMediaStoragePath(), "campaigns", key+".*"))
	if err != nil {
		return nil, "", fmt.Errorf("failed to locate staged media: %w", err)
	}
	if len(matches) == 0 {
		return nil, "", fmt.Errorf("staged media not found - it may have expired or already been used")
	}
	path := matches[0]
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read staged media: %w", err)
	}
	mimeType = detectCampaignMediaMimeType(filepath.Base(path), "", data)
	return data, mimeType, nil
}

// promoteRSVPReminderStagedMedia moves a staged reminder attachment onto its
// own campaign's media path and sets the fields the send actually depends on.
// Mirrors UploadCampaignMedia's two-phase update (campaigns.go:1657-1687):
// save the file, update the media info fields, then compute and persist the
// public URL from those fields (the token embedded in the URL is derived
// from them, so the URL must be built after they are final).
func (a *App) promoteRSVPReminderStagedMedia(campaign *models.BulkMessageCampaign, stagingID, stagingFilename, baseURL string) error {
	data, mimeType, err := a.loadStagedRSVPReminderMedia(stagingID)
	if err != nil {
		return err
	}
	localPath, err := a.saveCampaignMedia(campaign.ID.String(), data, mimeType)
	if err != nil {
		return fmt.Errorf("failed to save reminder media: %w", err)
	}
	filename := sanitizeFilename(stagingFilename)
	updates := map[string]interface{}{
		"header_media_id":         "",
		"header_media_filename":   filename,
		"header_media_mime_type":  mimeType,
		"header_media_local_path": localPath,
	}
	if err := a.DB.Model(campaign).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to save reminder media info: %w", err)
	}
	campaign.HeaderMediaID = ""
	campaign.HeaderMediaFilename = filename
	campaign.HeaderMediaMimeType = mimeType
	campaign.HeaderMediaLocalPath = localPath

	// HeaderMediaURL is what actually reaches Meta (worker.go:122); local path
	// alone renders only in the local chat bubble (worker.go:144-147).
	publicURL := a.buildCampaignMediaURLForBase(baseURL, campaign)
	if err := a.DB.Model(campaign).Update("header_media_url", publicURL).Error; err != nil {
		return fmt.Errorf("failed to save reminder media URL: %w", err)
	}
	campaign.HeaderMediaURL = publicURL
	return nil
}
