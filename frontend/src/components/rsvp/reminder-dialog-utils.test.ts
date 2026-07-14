import { describe, expect, it } from 'vitest'
import { responseCollection, responsePayload, templateParameterNames } from './reminder-dialog-utils'

describe('reminder dialog response helpers', () => {
  it('unwraps nested response collections and rejects malformed collections', () => {
    expect(responseCollection({ data: { data: { reminders: [{ id: 'one' }] } } }, 'reminders')).toEqual([{ id: 'one' }])
    expect(responseCollection({ data: { data: { reminders: 'not-an-array' } } }, 'reminders')).toEqual([])
    expect(responsePayload({ data: { data: ['unexpected'] } })).toEqual({})
  })

  it('extracts body and URL variables while tolerating malformed buttons', () => {
    expect(templateParameterNames({
      body_content: 'Hello {{1}} for {{event_name}}',
      buttons: [{ type: 'URL', url: 'https://example.test/{{member_phone}}' }, 'invalid'],
    })).toEqual(['1', 'event_name', 'member_phone'])
    expect(templateParameterNames({ body_content: null, buttons: {} })).toEqual([])
  })
})
