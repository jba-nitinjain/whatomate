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

// stagedRSVPReminderMediaPath locates the on-disk file UploadRSVPReminderMedia
// staged under stagingID. Staging ids are server-generated UUIDs
// (UploadRSVPReminderMedia never accepts a client-supplied id), so two staged
// uploads colliding on the same id - and thus this glob matching more than
// one file - is not reachable in practice; taking the first match needs no
// further tiebreak.
func (a *App) stagedRSVPReminderMediaPath(stagingID string) (string, error) {
	key := rsvpReminderStagingKey(stagingID)
	if key == "" {
		// A malformed staging id is not reachable from a normal client (see
		// rsvpReminderStagingKey), but if it happens the fix is the same as an
		// expired file: re-upload and send again. User-facing for the same
		// reason as the "not found" case below.
		return "", rsvpUserFacingError{fmt.Errorf("invalid media reference")}
	}
	matches, err := filepath.Glob(filepath.Join(a.getMediaStoragePath(), "campaigns", key+".*"))
	if err != nil {
		// A glob error here is a filesystem/infrastructure problem, not
		// something the user can fix by re-uploading - stays unwrapped.
		return "", fmt.Errorf("failed to locate staged media: %w", err)
	}
	if len(matches) == 0 {
		// The common case this wrapper exists for: the staged file expired or
		// was already cleaned up. The user can fix this by re-uploading, so it
		// must reach them as a 400 with this exact message, not a generic 500.
		return "", rsvpUserFacingError{fmt.Errorf("staged media not found - it may have expired or already been used")}
	}
	return matches[0], nil
}

// loadStagedRSVPReminderMedia reads back the bytes UploadRSVPReminderMedia
// staged under stagingID and derives their MIME type from the staged file
// itself, not from a client-supplied value. The send request previously
// carried a "staging_mime_type" field that was used to rebuild the staged
// file's extension for reading it; a mismatched echo of that field pointed
// HeaderMediaLocalPath at a file that does not exist. Locating the file by
// glob and re-detecting its type removes that trust boundary entirely.
func (a *App) loadStagedRSVPReminderMedia(stagingID string) (data []byte, mimeType string, err error) {
	path, err := a.stagedRSVPReminderMediaPath(stagingID)
	if err != nil {
		return nil, "", err
	}
	data, err = os.ReadFile(path)
	if err != nil {
		// stagedRSVPReminderMediaPath found a glob match a moment ago, but the
		// file can still vanish before the read (e.g. cleaned up concurrently) -
		// same user-fixable "re-upload" outcome as the not-found case above, so
		// this is wrapped the same way rather than falling through as a raw 500.
		return nil, "", rsvpUserFacingError{fmt.Errorf("failed to read staged media: %w", err)}
	}
	mimeType = detectCampaignMediaMimeType(filepath.Base(path), "", data)
	return data, mimeType, nil
}

// deleteStagedRSVPReminderMedia removes the on-disk staging file after its
// bytes have been copied to the campaign's own media path. Best-effort only:
// callers log a failure here rather than failing the send over a stray temp
// file - the campaign already has its own copy of the media by the time this
// runs.
func (a *App) deleteStagedRSVPReminderMedia(stagingID string) error {
	path, err := a.stagedRSVPReminderMediaPath(stagingID)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// promoteRSVPReminderStagedMedia copies a staged reminder attachment onto its
// own campaign's media path and sets the fields the send actually depends on.
// Mirrors UploadCampaignMedia's field handling (campaigns.go:1657-1687): save
// the file, set the media info fields, then compute the public URL from those
// fields (the token embedded in the URL is derived from them, so it must be
// built after they are final).
//
// This now runs BEFORE the campaign row exists (createRSVPReminderCampaign
// calls it ahead of the DB transaction, so that a template requiring media it
// can't attach fails before anything is persisted). campaign.ID is already
// assigned by the caller, which is all saveCampaignMedia and
// buildCampaignMediaURLForBase need - so all the fields set here are set only
// on the in-memory struct and ride along with the eventual tx.Create. There is
// no row yet to UPDATE.
func (a *App) promoteRSVPReminderStagedMedia(campaign *models.BulkMessageCampaign, stagingID, stagingFilename, baseURL string) error {
	data, mimeType, err := a.loadStagedRSVPReminderMedia(stagingID)
	if err != nil {
		return err
	}
	localPath, err := a.saveCampaignMedia(campaign.ID.String(), data, mimeType)
	if err != nil {
		return fmt.Errorf("failed to save reminder media: %w", err)
	}
	campaign.HeaderMediaID = ""
	campaign.HeaderMediaFilename = sanitizeFilename(stagingFilename)
	campaign.HeaderMediaMimeType = mimeType
	campaign.HeaderMediaLocalPath = localPath

	// HeaderMediaURL is what actually reaches Meta (worker.go:122); local path
	// alone renders only in the local chat bubble (worker.go:144-147).
	campaign.HeaderMediaURL = a.buildCampaignMediaURLForBase(baseURL, campaign)

	// The staged file has been copied to the campaign's own path above; the
	// staging copy is no longer needed. Do not fail the send over this -
	// campaigns.go:98's staged file is a temp upload, not the source of truth.
	if err := a.deleteStagedRSVPReminderMedia(stagingID); err != nil {
		a.Log.Warn("Failed to delete staged reminder media after promotion",
			"staging_id", stagingID, "error", err)
	}
	return nil
}
