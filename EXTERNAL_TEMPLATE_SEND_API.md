# External Template Send API

## Purpose

Use this endpoint when an external system wants Whatomate to send a WhatsApp template message directly.

This is different from `POST /api/messages/external`:

- `POST /api/messages/template` sends the message through WhatsApp
- `POST /api/messages/external` only stores a message that was already sent elsewhere

## Endpoint

```http
POST /api/messages/template
```

## Authentication

Use a normal API key or an authenticated session.

Example:

```http
X-API-Key: whm_your_api_key
Content-Type: application/json
```

If you are using a super admin API key for another organization, also send:

```http
X-Organization-ID: <target-org-uuid>
```

## Simple Request Contract

This is the recommended payload shape for external systems:

```json
{
  "phone_number": "919876543210",
  "phone_number_id": "116800811516809",
  "template_name": "credentials_v1",
  "template_params": {
    "name": "Nitin Jain",
    "portal": "Income Tax"
  },
  "header_media_url": "https://cdn.example.com/files/credentials.pdf",
  "header_media_filename": "credentials.pdf"
}
```

## Required fields

- `phone_number` or `contact_id`
- `template_name` or `template_id`

## Recommended fields for external systems

- `phone_number`
- `phone_number_id`
- `template_name`
- `template_params` when the template has dynamic values
- `header_media_url` when the template uses header media
- `header_media_filename` when the template uses a document header and the displayed document name should be controlled

## Field Notes

### Recipient

- `phone_number` sends directly to that number
- if the contact does not already exist, Whatomate creates it automatically
- `contact_id` can be used instead when the contact already exists in Whatomate

### WhatsApp account selection

Resolution priority is:

1. `phone_number_id`
2. `account_name`
3. the template's stored WhatsApp account
4. the contact's current WhatsApp account
5. the default outgoing account or fallback account

For external systems, `phone_number_id` is the simplest and safest option because it maps directly to the Meta phone number that should send the template.

### Template lookup

- `template_name` is the simplest option
- `template_id` can be used if your integration stores the local Whatomate template UUID
- the template must exist in Whatomate and be approved

### Dynamic parameters

Send dynamic values in `template_params`.

Named example:

```json
{
  "template_params": {
    "name": "Nitin Jain",
    "portal": "Income Tax"
  }
}
```

Positional example:

```json
{
  "template_params": {
    "1": "Nitin Jain",
    "2": "Income Tax"
  }
}
```

### Header media

If the template uses image, video, or document header media, you can send:

- `header_media_url`
- `header_media_id`
- multipart `header_file`
- `header_media_filename`

For most external systems, `header_media_url` is the simplest option.

### Document attachment name

For document-header templates, send the document name as:

```json
{
  "header_media_filename": "credentials.pdf"
}
```

Notes:

- use `header_media_filename`, not `document_name`, `file_name`, or `filename`
- with JSON requests, `header_media_filename` is used when Whatomate downloads `header_media_url` and uploads it to WhatsApp
- with multipart requests, `header_media_filename` can be sent as a normal text form field
- if `header_media_filename` is omitted for multipart, Whatomate uses the uploaded file part's filename
- if `header_media_filename` is omitted for `header_media_url`, Whatomate uses the basename from the URL path, for example `campaign-brief.pdf` from `https://cdn.example.com/files/campaign-brief.pdf?download=1`
- if `header_media_id` is used, Whatomate skips upload and uses the existing WhatsApp media ID; set the intended filename when that media is originally uploaded

## Common Examples

### 1. Minimal external send request

```bash
curl -X POST "http://localhost:8080/api/messages/template" \
  -H "X-API-Key: whm_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "919876543210",
    "phone_number_id": "116800811516809",
    "template_name": "credentials_v1",
    "template_params": {
      "name": "Nitin Jain",
      "portal": "Income Tax"
    }
  }'
```

## Invoice Approval Templates with Confirm / Reject Buttons

Use this when an external system needs to send an invoice approval message and receive the customer's button response back through an API call.

### What must exist first

Create and approve a WhatsApp template in Meta with quick reply buttons, for example:

- `Confirm`
- `Reject`

The customer will only see the button text. The invoice code can be sent as a hidden button payload.

### Send request

```bash
curl -X POST "http://localhost:8080/api/messages/template" \
  -H "X-API-Key: whm_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "919876543210",
    "phone_number_id": "116800811516809",
    "template_name": "invoice_approval_v1",
    "template_params": {
      "customer_name": "Nitin Jain",
      "invoice_amount": "5000"
    },
    "button_payloads": {
      "0": "invoice_code=INV-1001&action=confirm",
      "1": "invoice_code=INV-1001&action=reject"
    },
    "response_callback_url": "https://billing.example.com/api/invoice-response",
    "response_callback_bearer_token": "your-secret-token"
  }'
```

Notes:

- `button_payloads` keys are WhatsApp button positions: `0` for the first button, `1` for the second button, `2` for the third button.
- You may also use the button text as the key, for example `"Confirm": "invoice_code=INV-1001&action=confirm"`.
- `response_callback_url` is called by Whatomate after the customer taps a button.
- `response_callback_bearer_token` is sent as `Authorization: Bearer <token>`.

### Callback sent by Whatomate

When the customer taps a button, Whatomate sends a POST request to `response_callback_url`.

Example:

```json
{
  "invoice_code": "INV-1001",
  "action": "confirm",
  "button_payload": "invoice_code=INV-1001&action=confirm",
  "button_id": "0",
  "button_text": "Confirm",
  "contact_id": "4c0a7f45-7d2f-4a7f-9a3c-1b0f6c4e5e20",
  "contact_phone": "919876543210",
  "contact_name": "Nitin Jain",
  "whatsapp_account": "Main Account",
  "incoming_message_id": "6d2b7e17-42a1-4f54-9b7a-2abf1b283a11",
  "incoming_whatsapp_message_id": "wamid.incoming",
  "original_message_id": "12415f80-1d08-4c17-9030-08302bdf37a6",
  "original_whatsapp_message_id": "wamid.original",
  "timestamp": "2026-06-09T10:30:00Z"
}
```

### Callback behavior

- The incoming WhatsApp message is still saved even if the callback API fails.
- Whatomate stores callback status in the incoming message metadata for troubleshooting.
- Callback failures are not retried automatically in this version.
- Keep `response_callback_bearer_token` secret and rotate it if exposed.

### 2. With header media URL

```bash
curl -X POST "http://localhost:8080/api/messages/template" \
  -H "X-API-Key: whm_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "919876543210",
    "phone_number_id": "116800811516809",
    "template_name": "credentials_v1",
    "template_params": {
      "name": "Nitin Jain",
      "portal": "Income Tax"
    },
    "header_media_url": "https://cdn.example.com/template-header.jpg"
  }'
```

### 3. With document header name

```bash
curl -X POST "http://localhost:8080/api/messages/template" \
  -H "X-API-Key: whm_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "919876543210",
    "phone_number_id": "116800811516809",
    "template_name": "credentials_document_v1",
    "template_params": {
      "name": "Nitin Jain",
      "portal": "Income Tax"
    },
    "header_media_url": "https://cdn.example.com/files/generated-download?id=abc123",
    "header_media_filename": "income-tax-credentials.pdf"
  }'
```

### 4. Multipart document upload with document name

```bash
curl -X POST "http://localhost:8080/api/messages/template" \
  -H "X-API-Key: whm_your_api_key" \
  -F "phone_number=919876543210" \
  -F "phone_number_id=116800811516809" \
  -F "template_name=credentials_document_v1" \
  -F 'template_params={"name":"Nitin Jain","portal":"Income Tax"}' \
  -F "header_media_filename=income-tax-credentials.pdf" \
  -F "header_file=@/path/to/credentials.pdf;type=application/pdf"
```

### 5. Super admin API key for another organization

```bash
curl -X POST "http://localhost:8080/api/messages/template" \
  -H "X-API-Key: whm_your_super_admin_key" \
  -H "X-Organization-ID: 7d35e1f1-7b90-4b6c-8e70-6ef6a0e1c123" \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "919876543210",
    "phone_number_id": "116800811516809",
    "template_name": "credentials_v1",
    "template_params": {
      "name": "Nitin Jain",
      "portal": "Income Tax"
    }
  }'
```

## Success Response

Typical response:

```json
{
  "status": "success",
  "data": {
    "id": "uuid",
    "contact_id": "uuid",
    "direction": "outgoing",
    "message_type": "template",
    "content": {
      "body": "Hello Nitin Jain! Your portal is Income Tax."
    },
    "interactive_data": null,
    "status": "pending",
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
- `400 Either template_name or template_id is required`
- `400 Invalid contact_id`
- `400 Invalid template_id`
- `400 Missing template parameters: ...`
- `400 WhatsApp account not found for phone_number_id`
- `400 WhatsApp account not found`
- `404 Template not found`
- `404 Contact not found`
- `401 Unauthorized`
- `403 Insufficient permissions`

## Operational Notes

- this endpoint sends the message through WhatsApp
- if `phone_number` is new, Whatomate creates the contact before sending
- prefer `phone_number_id` for external systems so account routing is explicit
- use `template_name` unless your integration already stores local template UUIDs
