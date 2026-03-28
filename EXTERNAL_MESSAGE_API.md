# External Message API

## Purpose

`POST /api/messages/external` inserts an outbound message directly into Whatomate without sending it to the WhatsApp Cloud API.

Use this when another system has already sent the message and you only want Whatomate to:

- store the message in the conversation timeline
- update the contact's last-message preview
- associate the message with a WhatsApp account
- broadcast the new message to the UI/websocket clients

## What It Does

On success, the endpoint:

- resolves an existing contact or creates one from `phone_number`
- resolves the WhatsApp account to attach to the message
- stores a `messages` row with:
  - `direction = outgoing`
  - `status = sent`
  - `message_type = <type>`
- updates the contact's `last_message_at` and `last_message_preview`
- broadcasts the message into the application in real time

It does **not**:

- send anything to Meta / WhatsApp
- validate delivery with WhatsApp
- deduplicate by `external_message_id` or `whatsapp_message_id`

If the same payload is posted twice, two message records will be created.

## Authentication

Use normal authenticated app access or an API key.

Example:

```http
X-API-Key: whm_your_api_key
Content-Type: application/json
```

The caller must have:

- `chat:write`
- `contacts:read` when using `contact_id`
- `contacts:write` when using `phone_number` and allowing the API to create a contact

## Endpoint

```http
POST /api/messages/external
```

## Request Contract

### Required fields

- `type`
- one of `contact_id` or `phone_number`

### Supported body

```json
{
  "contact_id": "uuid",
  "phone_number": "919876543210",
  "phone_number_id": "116800811516809",
  "profile_name": "External Contact",
  "whatsapp_account": "primary",
  "type": "text",
  "content": {
    "body": "Sent from external system"
  },
  "media_url": "https://example.com/file.pdf",
  "media_mime_type": "application/pdf",
  "media_filename": "statement.pdf",
  "header_media_url": "https://example.com/template-header.jpg",
  "header_media_mime_type": "image/jpeg",
  "header_media_filename": "template-header.jpg",
  "interactive_data": {},
  "template_name": "order_update",
  "template_params": {
    "name": "Nitin",
    "order_id": "A-42"
  },
  "flow_response": {},
  "metadata": {
    "source_system": "external_system"
  },
  "whatsapp_message_id": "wamid.HBg...",
  "external_message_id": "crm-msg-123",
  "reply_to_message_id": "uuid",
  "sent_at": "2026-03-27T10:30:00Z"
}
```

## Field Notes

### Contact selection

- `contact_id`: uses an existing contact in the same organization
- `phone_number`: resolves or creates a contact
- when creating from `phone_number`, a leading `+` is stripped before storing
- `profile_name` is used when a new contact is created, and may also refresh the profile name on an existing matched contact

### WhatsApp account selection

- if `phone_number_id` is provided, the API resolves the WhatsApp account from that phone ID first
- if `whatsapp_account` is provided, that account is used
- otherwise the API tries the contact's current `whatsapp_account`
- if neither is set, Whatomate uses the default outgoing account, or any available account as fallback
- after account resolution, the contact is updated to that resolved account

### Message content

- for `text`, `content.body` is stored as the message content
- for `image`, `video`, `audio`, `document`, `interactive`, `flow`, `reaction`, `location`, `contact`, the endpoint stores the `type` plus any provided content/media fields
- for `document`, `media_filename` is useful because it affects the contact preview text

### Template messages

If `type = "template"`:

- the handler tries to find a local template by `template_name`
- if found, it renders `template.BodyContent` using `template_params`
- if rendering succeeds, the rendered text becomes the stored message content
- if `header_media_url`, `header_media_mime_type`, or `header_media_filename` is provided, that media is stored on the message so the template header can render in chat
- if the template has buttons and no `interactive_data` was supplied, interactive button metadata is generated
- if no template is found or rendering produces no text, the fallback content becomes:
  - `[Template: <template_name>]`
  - or `[Template]` if no name is present

This means template messages can still be inserted even if the local template record is missing.

### Metadata handling

- `metadata` is stored as-is
- if `metadata.source` is missing, the API adds `"source": "external_api"`
- if `external_message_id` is provided, it is also copied into `metadata.external_message_id`
- if `phone_number_id` is provided, it is copied into `metadata.phone_number_id` unless already present

### Message IDs

- `whatsapp_message_id` is stored in `messages.whats_app_message_id`
- `external_message_id` is only stored in `metadata`; it is not a unique key

### Reply messages

- `reply_to_message_id` must point to a message belonging to the same contact
- otherwise the request fails with `404 Reply-to message not found`

### Timestamp

- `sent_at` sets both `created_at` and `updated_at` on the inserted message
- if omitted, the server uses the current time

## Common Payloads

### 1. Text message

```json
{
  "phone_number": "919876543210",
  "profile_name": "Rahul",
  "whatsapp_account": "primary",
  "type": "text",
  "content": {
    "body": "Sent from external CRM"
  },
  "whatsapp_message_id": "wamid.external-123",
  "external_message_id": "crm-msg-123",
  "sent_at": "2026-03-27T10:30:00Z",
  "metadata": {
    "source_system": "crm"
  }
}
```

### 2. Template message

```json
{
  "contact_id": "9d7cb4e2-6e85-4f46-95bd-3ea4132c8123",
  "phone_number_id": "116800811516809",
  "whatsapp_account": "primary",
  "type": "template",
  "template_name": "credentials_v1",
  "template_params": {
    "name": "Nitin Jain",
    "portal": "Income Tax"
  },
  "content": {
    "body": "[Template: credentials_v1]"
  },
  "header_media_url": "https://cdn.example.com/template-header.jpg",
  "header_media_mime_type": "image/jpeg",
  "header_media_filename": "template-header.jpg",
  "whatsapp_message_id": "wamid.external-template-123"
}
```

### 3. Media message

```json
{
  "phone_number": "919876543210",
  "whatsapp_account": "primary",
  "type": "document",
  "content": {
    "body": "Monthly statement"
  },
  "media_url": "https://example.com/statement.pdf",
  "media_mime_type": "application/pdf",
  "media_filename": "statement.pdf",
  "whatsapp_message_id": "wamid.external-doc-123"
}
```

## Success Response

Typical response:

```json
{
  "status": "success",
  "data": {
    "id": "2d9f4bf4-8816-4f01-a4bf-2f1c8b22c5f8",
    "contact_id": "bf651630-00e2-46e7-8c73-cfb66bc9d338",
    "direction": "outgoing",
    "message_type": "text",
    "content": {
      "body": "Sent from external CRM"
    },
    "media_url": "",
    "media_mime_type": "",
    "media_filename": "",
    "interactive_data": null,
    "status": "sent",
    "wamid": "wamid.HBg...",
    "is_reply": false,
    "whatsapp_account": "primary",
    "created_at": "2026-03-27T10:30:00Z",
    "updated_at": "2026-03-27T10:30:00Z"
  }
}
```

## Error Cases

Common failures:

- `400 Either contact_id or phone_number is required`
- `400 type is required`
- `400 Invalid contact_id`
- `400 Invalid reply_to_message_id`
- `400 WhatsApp account not found`
- `400 no WhatsApp account configured`
- `401 Unauthorized`
- `403 Insufficient permissions`
- `403 You do not have permission to create contacts`
- `404 Contact not found`
- `404 Reply-to message not found`
- `500 Failed to resolve contact`
- `500 Failed to create message`

## cURL Example

```bash
curl -X POST "http://localhost:8080/api/messages/external" \
  -H "X-API-Key: whm_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "919876543210",
    "profile_name": "External Contact",
    "whatsapp_account": "primary",
    "type": "text",
    "content": {
      "body": "Sent from external CRM"
    },
    "external_message_id": "crm-msg-123",
    "whatsapp_message_id": "wamid.external-123",
    "metadata": {
      "source_system": "crm"
    },
    "sent_at": "2026-03-27T10:30:00Z"
  }'
```

## Operational Notes

- Treat this endpoint as a persistence API, not a delivery API.
- Only call it after your external sender has already accepted or sent the message.
- If you need retry safety, enforce idempotency on the caller side.
- If you want the conversation timeline to match WhatsApp accurately, always pass the real `whatsapp_message_id` returned by Meta.
