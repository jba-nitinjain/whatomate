/**
 * Union of answer keys across responses, in first-seen order — one column each.
 *
 * The chatbot writes both `<key>` (raw value, e.g. "yes") and `<key>_title`
 * (display value, e.g. "Attending"). Rendering both produced two columns with
 * the same label and different values, so the raw key is dropped wherever its
 * _title companion is present.
 */
export function visibleAnswerKeys(
  rows: Array<{ answers?: Record<string, unknown> }>,
): string[] {
  const seen: string[] = []
  for (const row of rows) {
    for (const k of Object.keys(row.answers || {})) {
      if (!k.startsWith('_') && !seen.includes(k)) seen.push(k)
    }
  }
  const titled = new Set(
    seen.filter(k => k.endsWith('_title')).map(k => k.slice(0, -'_title'.length)),
  )
  return seen.filter(k => !titled.has(k))
}
