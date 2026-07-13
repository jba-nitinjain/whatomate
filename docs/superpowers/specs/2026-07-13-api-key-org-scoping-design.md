# API Key Org Scoping & Super Admin Keys — Design

Date: 2026-07-13

## Problem

The API Keys create page (`frontend/src/views/settings/APIKeysView.vue`) never shows
which organisation a key is being created for. The backend already scopes key
creation correctly (org is derived server-side from the JWT, never from the
request body), so there is no data-leak bug — but there is no visual confirmation
for the user, and there is no way to create a platform-wide "super admin" API key
that isn't tied to a single organisation. Every row in `api_keys` currently
requires a non-null `organization_id`.

## Goals

1. Show the target organisation on the create-key page for regular users
   (read-only) and let super admins pick a target org, or opt into a
   platform-wide "Super Admin" key.
2. Support creating API keys that are not tied to any single organisation and
   carry full cross-org access when used, scoped to super admins only.
3. Surface org / key-type in the existing keys list, with super admins seeing
   keys across all organisations.
4. No change to non-super-admin behaviour or security posture — a regular user
   still cannot create or see keys outside their own org.

## Non-goals

- No changes to key hashing, expiry, or the `whm_` key format.
- No changes to RBAC permission checks (`api_keys:read/write/delete`) — these
  still gate the endpoints as they do today.

## Data model changes

`internal/models/models.go` — `APIKey` struct:

- `OrganizationID` becomes nullable: `*uuid.UUID` (was `uuid.UUID` with `not null`).
- New field `IsSuperAdminKey bool` (`gorm:"default:false;not null"`).
- Invariant: `OrganizationID == nil` if and only if `IsSuperAdminKey == true`.
  Enforced in the handler, not at the DB layer.

GORM AutoMigrate handles the column changes (no `.sql` migration file exists
today for this table, consistent with current pattern).

## Backend changes

### `internal/handlers/apikeys.go`

**`APIKeyRequest`** gains two optional fields:

```go
type APIKeyRequest struct {
    Name             string     `json:"name"`
    ExpiresAt        *string    `json:"expires_at,omitempty"`
    OrganizationID   *uuid.UUID `json:"organization_id,omitempty"`
    IsSuperAdminKey  bool       `json:"is_super_admin_key,omitempty"`
}
```

**`CreateAPIKey`**:
- Resolve caller's own org (`orgID`) and super-admin status as today.
- If caller is **not** a super admin: `OrganizationID`/`IsSuperAdminKey` in the
  request body are ignored outright; key is always created with
  `organization_id = orgID` (caller's own org), `is_super_admin_key = false`.
- If caller **is** a super admin:
  - `IsSuperAdminKey: true` → create with `organization_id = nil`,
    `is_super_admin_key = true`. Any `organization_id` in the body is ignored
    in this case.
  - `organization_id: <uuid>` provided → validate the org exists, create key
    scoped to that org.
  - Neither provided → default to caller's current org context (`orgID`), same
    as existing behaviour.

**`ListAPIKeys`**:
- Non-super-admin: unchanged — `WHERE organization_id = orgID`.
- Super admin: no org filter — returns all keys across all orgs plus
  super-admin keys. Preload `Organization` for name display; response adds
  `organization_name string \`json:"organization_name,omitempty"\`` and
  `is_super_admin_key bool` to `APIKeyResponse`. For a super-admin key,
  `organization_name` is omitted/empty and the frontend renders the "Super
  Admin" badge from `is_super_admin_key`.

**`DeleteAPIKey`**:
- Non-super-admin: unchanged — delete requires `organization_id = orgID` match.
- Super admin: may delete any key by ID regardless of org (drop the
  `organization_id` constraint from the `WHERE` clause when `IsSuperAdmin(userID)`
  is true).

### `internal/middleware/middleware.go` — `validateAPIKey`

- When the matched `APIKey.IsSuperAdminKey` is true:
  - Set `ContextKeyUserID`, `ContextKeyEmail`, `ContextKeyRoleID`,
    `ContextKeyIsSuperAdmin` from the key's creator user, as today.
  - Do **not** set `ContextKeyOrganizationID` from the key (there is none).
- Downstream, `getOrgID` (`internal/handlers/app.go:61`) already errors with
  "organization_id not found in context" when the context value is absent, and
  already supports super admins overriding org via the `X-Organization-ID`
  header. This means a super-admin key **must** pass `X-Organization-ID` on
  every org-scoped request — identical UX to today's super-admin JWT session
  behaviour. No new override mechanism is introduced; the existing one is
  reused as-is.
- Platform-level endpoints that don't require an org (e.g. listing
  organisations, managing users) continue to work with a super-admin key with
  no extra header needed, since they don't call `getOrgID`.

## Frontend changes

### `frontend/src/views/settings/APIKeysView.vue`

**Create dialog**:
- Regular users: read-only line "Creating key for: **{current org name}**"
  (org name pulled from the existing auth/org store — no new API call).
- Super admins: replace the read-only line with a `Select` populated from the
  existing `GET /api/organizations` endpoint (already super-admin gated). Add
  a leading option **"Super Admin (All Organizations)"**. Selecting it hides
  nothing else in the form; submitting sends `is_super_admin_key: true`
  instead of `organization_id`. Default selection = super admin's current org
  context (not the super-admin option), to avoid accidentally minting
  cross-org keys.

**List table**:
- New "Organization" column: org name for normal keys, amber `"Super Admin"`
  badge (reusing existing `Badge` component, similar styling to `is_active`
  badges) for `is_super_admin_key` rows.
- Column is populated for all users; for a regular user every row will simply
  show their own org's name.

**Usage hint dialog** (post-create key display): no changes needed.

## Security notes

- Privilege escalation is prevented the same way it is today: non-super-admin
  requests never have their `organization_id` / `is_super_admin_key` fields
  honoured, regardless of what the client sends.
- A super-admin key is exactly as powerful as a super-admin JWT session today
  (full cross-org access via `X-Organization-ID`) — no new privilege tier is
  introduced, only a new *credential type* carrying existing super-admin
  reach.
- Deleting/revoking a super-admin key is restricted to super admins, same as
  RBAC already requires for `api_keys:delete`.

## Testing

- Backend: extend `internal/handlers/apikeys_test.go` for: non-super-admin
  request body org fields ignored; super-admin creating org-specific key;
  super-admin creating platform key; list/delete scoping for both roles.
- Middleware: extend `internal/middleware/middleware_test.go` for a
  super-admin key request without `X-Organization-ID` failing org-scoped
  calls, and succeeding with it.
- Frontend: manual verification of both create-dialog variants and the new
  list column/badge.
