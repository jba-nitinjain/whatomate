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
  "header_media_url": "https://cdn.example.com/template-header.jpg"
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

For most external systems, `header_media_url` is the simplest option.

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

### 3. Super admin API key for another organization

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
