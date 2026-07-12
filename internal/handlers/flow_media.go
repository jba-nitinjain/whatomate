package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const flowMediaMaxSize = 16 << 20 // 16MB

// flowMediaToken signs a stored flow-media filename so the public serve route can
// validate access without exposing arbitrary files.
func flowMediaToken(secret, name string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(name))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// UploadFlowMedia stores an uploaded image/video/document and returns a public,
// token-signed URL usable as a chatbot media header (WhatsApp fetches it by link).
func (a *App) UploadFlowMedia(r *fastglue.Request) error {
	if _, err := a.getOrgID(r); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
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

	data, err := io.ReadAll(io.LimitReader(file, flowMediaMaxSize+1))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to read file", nil, "")
	}
	if len(data) > flowMediaMaxSize {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "File too large. Maximum size is 16MB", nil, "")
	}

	mimeType := detectCampaignMediaMimeType(fileHeader.Filename, fileHeader.Header.Get("Content-Type"), data)
	if !isAllowedCampaignMediaMimeType(mimeType) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Unsupported file type: "+mimeType, nil, "")
	}

	ext := getExtensionFromMimeType(mimeType)
	if ext == "" {
		ext = filepath.Ext(fileHeader.Filename)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to generate file name", nil, "")
	}
	name := hex.EncodeToString(buf) + ext

	if err := a.ensureMediaDir("flows"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to prepare storage", nil, "")
	}
	if err := os.WriteFile(filepath.Join(a.getMediaStoragePath(), "flows", name), data, 0o644); err != nil {
		a.Log.Error("Failed to save flow media", "error", err, "name", name)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to save media", nil, "")
	}

	token := flowMediaToken(a.Config.JWT.Secret, name)
	publicURL := fmt.Sprintf("%s/public/flow-media/%s?token=%s",
		a.requestPublicBaseURL(r.RequestCtx), url.PathEscape(name), url.QueryEscape(token))

	return r.SendEnvelope(map[string]interface{}{
		"media_url": publicURL,
		"mime_type": mimeType,
		"filename":  sanitizeFilename(fileHeader.Filename),
	})
}

// ServePublicFlowMedia streams a stored flow-media file after validating its token.
func (a *App) ServePublicFlowMedia(r *fastglue.Request) error {
	name, _ := r.RequestCtx.UserValue("filename").(string)
	name = filepath.Base(name) // defense against path traversal
	if name == "" || name == "." || strings.Contains(name, "/") {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid file", nil, "")
	}

	token := string(r.RequestCtx.QueryArgs().Peek("token"))
	expected := flowMediaToken(a.Config.JWT.Secret, name)
	if token == "" || !hmac.Equal([]byte(token), []byte(expected)) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Invalid or missing token", nil, "")
	}

	data, err := os.ReadFile(filepath.Join(a.getMediaStoragePath(), "flows", name))
	if err != nil {
		if os.IsNotExist(err) {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, "Media not found", nil, "")
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to read media", nil, "")
	}

	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	r.RequestCtx.SetContentType(contentType)
	r.RequestCtx.SetBody(data)
	return nil
}
