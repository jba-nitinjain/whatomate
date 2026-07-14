export interface ReminderTemplateLike {
  body_content?: unknown
  buttons?: unknown
}

export function responsePayload(response: any): Record<string, any> {
  const payload = response?.data?.data ?? response?.data ?? {}
  return payload && typeof payload === 'object' && !Array.isArray(payload) ? payload : {}
}

export function responseCollection<T>(response: any, key: string): T[] {
  const value = responsePayload(response)[key]
  return Array.isArray(value) ? value : []
}

export function templateParameterNames(template: ReminderTemplateLike | undefined): string[] {
  if (!template) return []

  const contents = [typeof template.body_content === 'string' ? template.body_content : '']
  const buttons = Array.isArray(template.buttons) ? template.buttons : []
  for (const value of buttons) {
    if (!value || typeof value !== 'object') continue
    const button = value as Record<string, unknown>
    if (String(button.type || '').toUpperCase() === 'URL' && typeof button.url === 'string') {
      contents.push(button.url)
    }
  }

  const matches = contents.join('\n').match(/\{\{([^}]+)\}\}/g) || []
  return [...new Set(matches.map(value => value.replace(/\{\{|\}\}/g, '').trim()).filter(Boolean))]
}
