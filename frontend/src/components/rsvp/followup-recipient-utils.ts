// Pure helpers for RSVPFollowUpDialog.vue's recipient list, split out (same
// spirit as reminder-dialog-utils.ts) so search/pagination logic is testable
// without mounting the dialog. Unlike RSVPReminderDialog.vue's recipient
// list - which pages through a server endpoint - the follow-up preview
// (GET /followup/preview) already returns every eligible recipient in one
// response, so this filters/paginates that in-memory list on the client.

export interface FollowUpRecipient {
  id: string
  phone_number: string
  contact?: { profile_name?: string } | null
}

export function isFollowUpRecipient(value: unknown): value is FollowUpRecipient {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  return typeof record.id === 'string' && typeof record.phone_number === 'string'
}

export function followUpRecipientLabel(recipient: FollowUpRecipient): string {
  return recipient.contact?.profile_name || recipient.phone_number
}

// Matches by name or phone number, case-insensitively - mirrors the
// server-side "search" behaviour RSVPReminderDialog.vue relies on
// (rsvpService.guests({ search })), reimplemented client-side since the
// follow-up preview has no search param.
export function filterFollowUpRecipients(recipients: FollowUpRecipient[], search: string): FollowUpRecipient[] {
  const query = search.trim().toLowerCase()
  if (!query) return recipients
  return recipients.filter(recipient => {
    const name = (recipient.contact?.profile_name || '').toLowerCase()
    const phone = recipient.phone_number.toLowerCase()
    return name.includes(query) || phone.includes(query)
  })
}

export function paginateFollowUpRecipients(recipients: FollowUpRecipient[], page: number, limit: number): FollowUpRecipient[] {
  if (limit <= 0) return []
  const safePage = Math.max(1, Math.floor(page) || 1)
  const start = (safePage - 1) * limit
  return recipients.slice(start, start + limit)
}

export function followUpRecipientPageCount(total: number, limit: number): number {
  if (limit <= 0) return 1
  return Math.max(1, Math.ceil(total / limit))
}
