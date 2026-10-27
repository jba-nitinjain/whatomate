// Campaign date helpers. Display is always dd/mm/yyyy HH:mm (local time);
// values are stored/transmitted as ISO strings.

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

export function formatCampaignDateTime(value?: string | null): string {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// toCampaignDateTimeLocal converts an ISO timestamp into the value format of an
// <input type="datetime-local"> in the viewer's local time zone.
export function toCampaignDateTimeLocal(value?: string | null): string {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// toCampaignScheduledAt converts a datetime-local value into an ISO timestamp,
// or null to clear the schedule.
export function toCampaignScheduledAt(value: string): string | null {
  if (!value) return null
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return null
  return d.toISOString()
}
