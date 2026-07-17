import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en.json'

describe('RSVP follow-up translations', () => {
  it('renders the live audience count without an i18n syntax error', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'en',
      messages: { en },
    })

    expect(i18n.global.t('rsvp.followUpCount', { count: 271 })).toBe('271 guests will be asked')
    expect(i18n.global.t('rsvp.followUpSend', { count: 271 })).toBe('Send follow-up (271)')
    expect(i18n.global.t('rsvp.followUpCampaignCreated', { queued: 271, skipped: 3 }))
      .toBe('Follow-up campaign queued for 271 recipients; 3 skipped.')
  })
})
