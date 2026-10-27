package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nikyjain/whatomate/internal/templateutil"
)

// truncateLabel caps a customer-visible button label at max characters.
// Meta counts characters, not bytes, and the locales this app ships (hi/ta/ar)
// are multi-byte — slicing on a byte boundary splits a rune and the label
// arrives garbled instead of merely shortened.
func truncateLabel(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// SendTextMessage sends a text message to a recipient with optional reply context
func (c *Client) SendTextMessage(ctx context.Context, account *Account, rcpt Recipient, text string, replyToMsgID ...string) (string, error) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "text",
		"text": map[string]any{
			"preview_url": false,
			"body":        text,
		},
	}
	rcpt.SetOnPayload(payload)

	// Add reply context if provided
	if len(replyToMsgID) > 0 && replyToMsgID[0] != "" {
		payload["context"] = map[string]any{
			"message_id": replyToMsgID[0],
		}
	}

	url := c.buildMessagesURL(account)
	c.Log.Debug("Sending text message", "phone", rcpt.Phone, "url", url)

	respBody, err := c.doRequest(ctx, "POST", url, payload, account.AccessToken)
	if err != nil {
		c.Log.Error("Failed to send text message", "error", err, "phone", rcpt.Phone)
		return "", fmt.Errorf("failed to send text message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.Log.Info("Text message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// SendInteractiveButtons sends reply buttons when there are up to 3 options and
// falls back to a list for 4-10 options to preserve legacy caller behavior.
func (c *Client) SendInteractiveButtons(ctx context.Context, account *Account, rcpt Recipient, bodyText string, buttons []Button) (string, error) {
	if len(buttons) <= 3 {
		return c.SendInteractiveReplyButtons(ctx, account, rcpt, bodyText, buttons)
	}
	return c.SendInteractiveList(ctx, account, rcpt, bodyText, "Select an option", "Options", buttons)
}

// SendInteractiveReplyButtons sends an explicit WhatsApp reply-button message.
func (c *Client) SendInteractiveReplyButtons(ctx context.Context, account *Account, rcpt Recipient, bodyText string, buttons []Button) (string, error) {
	return c.SendInteractiveReplyButtonsWithHeader(ctx, account, rcpt, bodyText, buttons, "", "")
}

// SendInteractiveReplyButtonsWithHeader sends a reply-button message with an
// optional media header (image/video/document) shown above the body, so the flow
// can present a poster/clip and tappable buttons in a single message.
func (c *Client) SendInteractiveReplyButtonsWithHeader(ctx context.Context, account *Account, rcpt Recipient, bodyText string, buttons []Button, headerType, headerLink string) (string, error) {
	if len(buttons) == 0 {
		return "", fmt.Errorf("at least one button is required")
	}
	if len(buttons) > 3 {
		return "", fmt.Errorf("maximum 3 reply buttons allowed")
	}

	buttonsList := make([]map[string]any, 0, len(buttons))
	for _, btn := range buttons {
		title := truncateLabel(btn.Title, 20)
		buttonsList = append(buttonsList, map[string]any{
			"type": "reply",
			"reply": map[string]any{
				"id":    btn.ID,
				"title": title,
			},
		})
	}

	interactive := map[string]any{
		"type": "button",
		"body": map[string]any{
			"text": bodyText,
		},
		"action": map[string]any{
			"buttons": buttonsList,
		},
	}
	if link := strings.TrimSpace(headerLink); link != "" {
		ht := strings.ToLower(strings.TrimSpace(headerType))
		switch ht {
		case "image", "video", "document":
		default:
			ht = "image"
		}
		interactive["header"] = map[string]any{
			"type": ht,
			ht:     map[string]any{"link": link},
		}
	}
	return c.sendInteractivePayload(ctx, account, rcpt, interactive, len(buttons))
}

// SendInteractiveList sends an explicit WhatsApp list-picker message.
func (c *Client) SendInteractiveList(ctx context.Context, account *Account, rcpt Recipient, bodyText, buttonText, sectionTitle string, buttons []Button) (string, error) {
	if len(buttons) == 0 {
		return "", fmt.Errorf("at least one list row is required")
	}
	if len(buttons) > 10 {
		return "", fmt.Errorf("maximum 10 buttons allowed")
	}
	if buttonText == "" {
		buttonText = "Select an option"
	}
	if sectionTitle == "" {
		sectionTitle = "Options"
	}

	rows := make([]map[string]any, 0, len(buttons))
	for _, btn := range buttons {
		title := truncateLabel(btn.Title, 24)
		row := map[string]any{
			"id":    btn.ID,
			"title": title,
		}
		if btn.Description != "" {
			row["description"] = truncateLabel(btn.Description, 72)
		}
		rows = append(rows, row)
	}

	interactive := map[string]any{
		"type": "list",
		"body": map[string]any{
			"text": bodyText,
		},
		"action": map[string]any{
			"button": buttonText,
			"sections": []map[string]any{
				{
					"title": sectionTitle,
					"rows":  rows,
				},
			},
		},
	}
	return c.sendInteractivePayload(ctx, account, rcpt, interactive, len(buttons))
}

func (c *Client) sendInteractivePayload(ctx context.Context, account *Account, rcpt Recipient, interactive map[string]any, optionCount int) (string, error) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "interactive",
		"interactive":       interactive,
	}
	rcpt.SetOnPayload(payload)

	url := c.buildMessagesURL(account)
	c.Log.Debug("Sending interactive message", "phone", rcpt.Phone, "option_count", optionCount)

	respBody, err := c.doRequest(ctx, "POST", url, payload, account.AccessToken)
	if err != nil {
		c.Log.Error("Failed to send interactive message", "error", err, "phone", rcpt.Phone)
		return "", fmt.Errorf("failed to send interactive message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.Log.Info("Interactive message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// SendMediaByLink sends an image/video/document message by public URL (no upload
// step) with an optional caption. mediaType is "image", "video" or "document".
func (c *Client) SendMediaByLink(ctx context.Context, account *Account, rcpt Recipient, mediaType, link, caption string) (string, error) {
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	switch mt {
	case "image", "video", "document":
	default:
		return "", fmt.Errorf("unsupported media type %q", mediaType)
	}
	media := map[string]any{"link": link}
	if strings.TrimSpace(caption) != "" {
		media["caption"] = caption
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              mt,
		mt:                  media,
	}
	rcpt.SetOnPayload(payload)
	apiURL := c.buildMessagesURL(account)
	respBody, err := c.doRequest(ctx, "POST", apiURL, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send media message: %w", err)
	}
	var resp MetaAPIResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("failed to parse media response: %w", err)
	}
	if len(resp.Messages) == 0 {
		return "", fmt.Errorf("no message ID in media response")
	}
	return resp.Messages[0].ID, nil
}

// SendCTAURLButton sends an interactive message with a CTA URL button
// This opens a URL when clicked instead of sending a reply
func (c *Client) SendCTAURLButton(ctx context.Context, account *Account, rcpt Recipient, bodyText, buttonText, url string) (string, error) {
	if buttonText == "" || url == "" {
		return "", fmt.Errorf("button text and URL are required")
	}

	// Truncate button text to 20 chars (WhatsApp limit)
	buttonText = truncateLabel(buttonText, 20)

	interactive := map[string]any{
		"type": "cta_url",
		"body": map[string]any{
			"text": bodyText,
		},
		"action": map[string]any{
			"name": "cta_url",
			"parameters": map[string]any{
				"display_text": buttonText,
				"url":          url,
			},
		},
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "interactive",
		"interactive":       interactive,
	}
	rcpt.SetOnPayload(payload)

	apiURL := c.buildMessagesURL(account)
	c.Log.Debug("Sending CTA URL button message", "phone", rcpt.Phone, "url", url)

	respBody, err := c.doRequest(ctx, "POST", apiURL, payload, account.AccessToken)
	if err != nil {
		c.Log.Error("Failed to send CTA URL button message", "error", err, "phone", rcpt.Phone)
		return "", fmt.Errorf("failed to send CTA URL button message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.Log.Info("CTA URL button message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// SendVoiceCallButton sends an interactive message with a WhatsApp Business
// Calling voice_call button. When the recipient taps the button, Meta
// initiates a voice call back to our number; the resulting incoming-call
// webhook echoes the `payload` string back as `biz_opaque_callback_data`, so
// callers can use it for routing (e.g. sticky-assigning the call to the
// agent who sent the button).
//
// ttlMinutes is how long the button remains clickable; pass 0 to use Meta's
// default (15 min). The sending phone number must be enrolled in the
// WhatsApp Business Calling API or Meta rejects the send.
func (c *Client) SendVoiceCallButton(ctx context.Context, account *Account, rcpt Recipient, bodyText, displayText string, ttlMinutes int, payload string) (string, error) {
	if bodyText == "" {
		return "", fmt.Errorf("body text is required")
	}
	if displayText == "" {
		return "", fmt.Errorf("display text is required")
	}
	displayText = truncateLabel(displayText, 20)

	parameters := map[string]any{
		"display_text": displayText,
	}
	if ttlMinutes > 0 {
		parameters["ttl_minutes"] = ttlMinutes
	}
	if payload != "" {
		parameters["payload"] = payload
	}

	interactive := map[string]any{
		"type": "voice_call",
		"body": map[string]any{
			"text": bodyText,
		},
		"action": map[string]any{
			"name":       "voice_call",
			"parameters": parameters,
		},
	}

	msg := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "interactive",
		"interactive":       interactive,
	}
	rcpt.SetOnPayload(msg)

	url := c.buildMessagesURL(account)
	// Logged at info during the sticky-routing rollout: confirms display_text,
	// ttl_minutes, and the agent-id payload actually leave our box, so when
	// the incoming-call webhook arrives we know whether Meta echoed it back.
	// The payload is an opaque "agent:<uuid>" — not PII.
	c.Log.Info("Sending voice_call button message",
		"phone", rcpt.Phone,
		"parameters", parameters,
	)

	respBody, err := c.doRequest(ctx, "POST", url, msg, account.AccessToken)
	if err != nil {
		c.Log.Error("Failed to send voice_call button message", "error", err, "phone", rcpt.Phone)
		return "", fmt.Errorf("failed to send voice_call button message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.Log.Info("voice_call button message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// TemplateParam represents a parameter for template message
type TemplateParam struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Image *struct {
		Link string `json:"link"`
	} `json:"image,omitempty"`
	Document *struct {
		Link     string `json:"link"`
		Filename string `json:"filename"`
	} `json:"document,omitempty"`
	Video *struct {
		Link string `json:"link"`
	} `json:"video,omitempty"`
}

// SendTemplateMessage sends a template message
// sortParamKeys returns the keys of paramMap in the order they should be sent
// to Meta. Named templates (forceLexical=true, or any non-numeric key) sort
// lexicographically. Otherwise keys are treated as positional indices and
// sorted numerically — required so that "1","2",..,"10","11" stay in order
// instead of becoming "1","10","11",..,"2","9".
func sortParamKeys(paramMap map[string]string, forceLexical bool) []string {
	keys := make([]string, 0, len(paramMap))
	for k := range paramMap {
		keys = append(keys, k)
	}
	if forceLexical {
		sort.Strings(keys)
		return keys
	}
	for _, k := range keys {
		if _, err := strconv.Atoi(k); err != nil {
			// Mixed/named keys — fall back to lexical to keep behaviour stable.
			sort.Strings(keys)
			return keys
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, _ := strconv.Atoi(keys[i])
		nj, _ := strconv.Atoi(keys[j])
		return ni < nj
	})
	return keys
}

// BodyParamsToComponents converts a bodyParams map into WhatsApp template components.
// Supports both positional (numeric keys) and named parameters.
func BodyParamsToComponents(bodyParams map[string]string) []map[string]any {
	if len(bodyParams) == 0 {
		return nil
	}

	// Check if using named parameters (non-numeric keys like "name", "order_id")
	isNamedParams := false
	for key := range bodyParams {
		if _, err := strconv.Atoi(key); err != nil {
			isNamedParams = true
			break
		}
	}

	// Get sorted keys for deterministic ordering. For positional templates the
	// keys are numeric strings ("1".."14") and MUST be ordered numerically —
	// sort.Strings would yield "1","10","11",..,"2",..,"9" and ship parameters
	// to Meta in the wrong slot, so {{2}}..{{9}} render as the values that
	// belonged in {{10}}+ on the recipient's device (issue #354).
	keys := sortParamKeys(bodyParams, isNamedParams)

	params := make([]map[string]any, 0, len(bodyParams))
	for _, key := range keys {
		param := map[string]any{
			"type": "text",
			"text": bodyParams[key],
		}
		if isNamedParams {
			param["parameter_name"] = key
		}
		params = append(params, param)
	}

	return []map[string]any{
		{
			"type":       "body",
			"parameters": params,
		},
	}
}

// HeaderTextParamsComponent builds the header component for a TEXT header that
// contains one variable. Meta restricts TEXT headers to at most one variable,
// so this returns an error if `headerContent` has more.
//
// `params` is the caller's value map (e.g. {"order_id": "ORD-1"} or {"1": "ORD-1"}).
// `fallback` is consulted if the value isn't found in `params` — useful for
// callers that share one flat map across header + body.
//
// Returns (nil, nil) when the header has no variable.
func HeaderTextParamsComponent(headerContent string, params, fallback map[string]string) (map[string]any, error) {
	if !strings.Contains(headerContent, "{{") {
		return nil, nil
	}
	names := templateutil.ExtParamNames(headerContent)
	if len(names) == 0 {
		return nil, nil
	}
	if len(names) > 1 {
		return nil, fmt.Errorf("header text may contain at most one variable; found %d", len(names))
	}

	name := names[0]
	value := params[name]
	if value == "" {
		value = fallback[name]
	}

	param := map[string]any{
		"type": "text",
		"text": value,
	}
	// Named params include parameter_name; positional ("1") don't.
	if _, err := strconv.Atoi(name); err != nil {
		param["parameter_name"] = name
	}

	return map[string]any{
		"type":       "header",
		"parameters": []map[string]any{param},
	}, nil
}

// BuildTemplateComponents builds the full WhatsApp template components array,
// including an optional header component (TEXT with variable, or IMAGE/VIDEO/
// DOCUMENT media) and body parameters.
//
// headerMediaFilename is required by Meta for DOCUMENT headers — without it, the
// API returns error 132012 "Header Format Mismatch (Expected DOCUMENT, received
// UNKNOWN)". It is ignored for IMAGE/VIDEO.
//
// headerContent and headerParams are only consulted for TEXT headers. For media
// headers (IMAGE/VIDEO/DOCUMENT) the existing headerMediaID/Filename path is used.
// Returns an error if the TEXT header declares more than one variable.
func BuildTemplateComponents(
	bodyParams map[string]string,
	headerType, headerContent string,
	headerParams map[string]string,
	headerMediaID, headerMediaFilename string,
) ([]map[string]any, error) {
	if headerMediaID == "" {
		headerMediaFilename = ""
	}
	return BuildTemplateComponentsWithHeaderText(bodyParams, nil, nil, nil, headerType, headerContent, headerParams, headerMediaID, headerMediaFilename)
}

// BuildTemplateComponentsWithHeaderText is the full template component builder:
// an optional TEXT header parameter (when the approved header declares a
// {{var}}), media header, body parameters, dynamic URL button parameters and
// QUICK_REPLY payloads. Media arguments are ignored for TEXT headers.
func BuildTemplateComponentsWithHeaderText(bodyParams map[string]string, buttonParams map[int]string, quickReplyPayloads map[int]string, buttons []any, headerType, headerContent string, headerParams map[string]string, headerMediaID, headerMediaLink string) ([]map[string]any, error) {
	var components []map[string]any
	if strings.EqualFold(headerType, "TEXT") {
		headerComp, err := HeaderTextParamsComponent(headerContent, headerParams, bodyParams)
		if err != nil {
			return nil, err
		}
		if headerComp != nil {
			components = append(components, headerComp)
		}
		headerMediaID, headerMediaLink = "", ""
	}
	components = append(components, BuildTemplateComponentsWithQuickReplyPayloads(bodyParams, buttonParams, quickReplyPayloads, buttons, headerType, headerMediaID, headerMediaLink)...)
	if len(components) == 0 {
		return nil, nil
	}
	return components, nil
}

// BuildTemplateComponentsWithQuickReplyPayloads builds template components including
// an optional media header, body parameters, dynamic URL button parameters
// (buttonParams, keyed by button index) and Meta-supported hidden payloads for
// QUICK_REPLY template buttons. Header media prefers an uploaded media ID and
// falls back to a public link. headerMediaLink doubles as the document filename
// source when a media ID is supplied (a bare filename is accepted).
func BuildTemplateComponentsWithQuickReplyPayloads(bodyParams map[string]string, buttonParams map[int]string, quickReplyPayloads map[int]string, buttons []any, headerType, headerMediaID, headerMediaLink string) []map[string]any {
	var components []map[string]any

	// Add header component if media is provided
	if headerMediaID != "" || headerMediaLink != "" {
		mediaType := strings.ToLower(headerType) // "image", "video", "document"
		mediaObj := map[string]any{}
		if headerMediaID != "" {
			mediaObj["id"] = headerMediaID
			if mediaType == "document" && headerMediaLink != "" {
				if filename := templateHeaderFilenameFromLink(headerMediaLink); filename != "" {
					mediaObj["filename"] = filename
				} else {
					mediaObj["filename"] = headerMediaLink
				}
			}
		} else {
			mediaObj["link"] = headerMediaLink
			if mediaType == "document" {
				if filename := templateHeaderFilenameFromLink(headerMediaLink); filename != "" {
					mediaObj["filename"] = filename
				}
			}
		}
		headerParam := map[string]any{
			"type":    mediaType,
			mediaType: mediaObj,
		}
		components = append(components, map[string]any{
			"type":       "header",
			"parameters": []map[string]any{headerParam},
		})
	}

	// Add body component with text parameters
	bodyComponents := BodyParamsToComponents(bodyParams)
	components = append(components, bodyComponents...)

	// Add button components for URL buttons with dynamic placeholders and
	// quick-reply buttons with hidden payloads.
	for idx, button := range buttons {
		btnMap, ok := button.(map[string]any)
		if !ok {
			continue
		}
		btnType, _ := btnMap["type"].(string)
		switch strings.ToUpper(btnType) {
		case "URL":
			paramValue := buttonParams[idx]
			if paramValue == "" {
				continue
			}
			components = append(components, map[string]interface{}{
				"type":     "button",
				"sub_type": "url",
				"index":    fmt.Sprintf("%d", idx),
				"parameters": []map[string]interface{}{
					{
						"type": "text",
						"text": paramValue,
					},
				},
			})
		case "QUICK_REPLY":
			payload := quickReplyPayloads[idx]
			if payload == "" {
				continue
			}
			components = append(components, map[string]interface{}{
				"type":     "button",
				"sub_type": "quick_reply",
				"index":    fmt.Sprintf("%d", idx),
				"parameters": []map[string]interface{}{
					{
						"type":    "payload",
						"payload": payload,
					},
				},
			})
		default:
			continue
		}
	}

	if len(components) == 0 {
		return nil
	}
	return components
}

func templateHeaderFilenameFromLink(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name := path.Base(parsed.Path)
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return name
}

// AutoButtonComponents generates button components for button types that require
// server-generated parameters (FLOW needs flow_token, OTP needs the code).
// These are auto-generated and don't require user input.
func AutoButtonComponents(templateButtons []any) []map[string]any {
	if len(templateButtons) == 0 {
		return nil
	}

	var components []map[string]any
	for i, raw := range templateButtons {
		btn, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		t, _ := btn["type"].(string)
		t = strings.ToUpper(t)

		switch t {
		case "FLOW":
			components = append(components, map[string]any{
				"type":     "button",
				"sub_type": "flow",
				"index":    fmt.Sprintf("%d", i),
				"parameters": []map[string]any{
					{
						"type": "action",
						"action": map[string]any{
							"flow_token": fmt.Sprintf("flow_%d", time.Now().UnixNano()),
						},
					},
				},
			})
		}
	}
	return components
}

// ButtonURLParamsToComponents converts button parameters to WhatsApp API button components.
// buttonParams maps button index (as string like "0", "1") to the dynamic parameter value.
// templateButtons is the JSONB buttons array from the template, used to determine button type.
// URL buttons produce: {"type": "button", "sub_type": "url", "index": "0", "parameters": [{"type": "text", "text": "value"}]}
// COPY_CODE buttons produce: {"type": "button", "sub_type": "copy_code", "index": "0", "parameters": [{"type": "coupon_code", "coupon_code": "value"}]}
func ButtonURLParamsToComponents(buttonParams map[string]string, templateButtons ...[]any) []map[string]any {
	if len(buttonParams) == 0 {
		return nil
	}

	// Build a lookup of button index -> effective type from template buttons.
	// OTP buttons resolve to their otp_type (COPY_CODE, ONE_TAP, ZERO_TAP)
	// so the message sending logic handles them correctly.
	// btnIsOTP tracks whether the button was originally an OTP button (auth templates
	// need sub_type "url" instead of "copy_code").
	btnTypes := map[string]string{}
	btnIsOTP := map[string]bool{}
	if len(templateButtons) > 0 {
		for i, raw := range templateButtons[0] {
			if btn, ok := raw.(map[string]any); ok {
				if t, ok := btn["type"].(string); ok {
					key := fmt.Sprintf("%d", i)
					effectiveType := strings.ToUpper(t)
					if effectiveType == "OTP" {
						btnIsOTP[key] = true
						if otpType, ok := btn["otp_type"].(string); ok {
							effectiveType = strings.ToUpper(otpType)
						}
					}
					btnTypes[key] = effectiveType
				}
			}
		}
	}

	// Button indices are always numeric strings ("0", "1", ...) so sort
	// numerically — same lexical-sort hazard as positional body params.
	keys := sortParamKeys(buttonParams, false)

	components := make([]map[string]any, 0, len(buttonParams))
	for _, index := range keys {
		value := buttonParams[index]
		// Skip button types that don't accept dynamic parameters
		if t := btnTypes[index]; t == "QUICK_REPLY" || t == "FLOW" || t == "PHONE_NUMBER" || t == "VOICE_CALL" || t == "ONE_TAP" || t == "ZERO_TAP" {
			continue
		}
		if btnTypes[index] == "COPY_CODE" && !btnIsOTP[index] {
			// Regular COPY_CODE button (e.g. coupon codes)
			components = append(components, map[string]any{
				"type":     "button",
				"sub_type": "copy_code",
				"index":    index,
				"parameters": []map[string]any{
					{"type": "coupon_code", "coupon_code": value},
				},
			})
		} else {
			components = append(components, map[string]any{
				"type":     "button",
				"sub_type": "url",
				"index":    index,
				"parameters": []map[string]any{
					{"type": "text", "text": value},
				},
			})
		}
	}
	return components
}

// SendFlowMessage sends an interactive WhatsApp Flow message
// flowID is the Meta Flow ID, headerText is optional header, bodyText is the message body,
// ctaText is the button text, flowToken is a unique token for tracking the flow response,
// and firstScreen is the name of the first screen to navigate to
func (c *Client) SendFlowMessage(ctx context.Context, account *Account, rcpt Recipient, flowID, headerText, bodyText, ctaText, flowToken, firstScreen string) (string, error) {
	if flowID == "" {
		return "", fmt.Errorf("flow ID is required")
	}
	if bodyText == "" {
		return "", fmt.Errorf("body text is required")
	}
	if ctaText == "" {
		ctaText = "Open" // Default CTA text
	}
	if flowToken == "" {
		flowToken = fmt.Sprintf("flow_%d", time.Now().UnixNano())
	}
	if firstScreen == "" {
		firstScreen = "FIRST_SCREEN" // Default fallback
	}

	// Truncate CTA text to 20 chars (WhatsApp limit)
	ctaText = truncateLabel(ctaText, 20)

	interactive := map[string]any{
		"type": "flow",
		"body": map[string]any{
			"text": bodyText,
		},
		"action": map[string]any{
			"name": "flow",
			"parameters": map[string]any{
				"flow_message_version": "3",
				"flow_token":           flowToken,
				"flow_id":              flowID,
				"flow_cta":             ctaText,
				"flow_action":          "navigate",
				"flow_action_payload": map[string]any{
					"screen": firstScreen,
				},
			},
		},
	}

	// Add header if provided
	if headerText != "" {
		interactive["header"] = map[string]any{
			"type": "text",
			"text": headerText,
		}
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "interactive",
		"interactive":       interactive,
	}
	rcpt.SetOnPayload(payload)

	url := c.buildMessagesURL(account)
	c.Log.Debug("Sending flow message", "phone", rcpt.Phone, "flow_id", flowID)

	respBody, err := c.doRequest(ctx, "POST", url, payload, account.AccessToken)
	if err != nil {
		c.Log.Error("Failed to send flow message", "error", err, "phone", rcpt.Phone, "flow_id", flowID)
		return "", fmt.Errorf("failed to send flow message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.Log.Info("Flow message sent", "message_id", messageID, "phone", rcpt.Phone, "flow_id", flowID)
	return messageID, nil
}

// SendTemplateMessage sends a template message with optional components (header, body, buttons, etc.)
func (c *Client) SendTemplateMessage(ctx context.Context, account *Account, rcpt Recipient, templateName, languageCode string, components []map[string]any) (string, error) {
	template := map[string]any{
		"name": templateName,
		"language": map[string]any{
			"code": languageCode,
		},
	}

	if len(components) > 0 {
		template["components"] = components
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"type":              "template",
		"template":          template,
	}
	rcpt.SetOnPayload(payload)

	url := c.buildMessagesURL(account)
	c.Log.Debug("Sending template message with components", "phone", rcpt.Phone, "template", templateName)

	respBody, err := c.doRequest(ctx, "POST", url, payload, account.AccessToken)
	if err != nil {
		c.Log.Error("Failed to send template message", "error", err, "phone", rcpt.Phone, "template", templateName)
		return "", fmt.Errorf("failed to send template message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.Log.Info("Template message sent", "message_id", messageID, "phone", rcpt.Phone, "template", templateName)
	return messageID, nil
}

// SendMarketingTemplateMessage sends a marketing template through the MM Lite-compatible path.
// Meta currently documents MM Lite as an onboarding/routing layer for marketing templates,
// so the payload remains a standard WhatsApp template message.
func (c *Client) SendMarketingTemplateMessage(ctx context.Context, account *Account, rcpt Recipient, templateName, languageCode string, components []map[string]any) (string, error) {
	c.Log.Info("Sending marketing template message", "phone", rcpt.Phone, "template", templateName, "route", "marketing_messages_lite")
	return c.SendTemplateMessage(ctx, account, rcpt, templateName, languageCode, components)
}
