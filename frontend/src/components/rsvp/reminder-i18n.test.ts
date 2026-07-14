import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en.json'

describe('RSVP reminder translations', () => {
  it('renders the dynamic token example without an i18n syntax error', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'en',
      messages: { en },
    })

    expect(i18n.global.t('rsvp.reminderVariablePlaceholder', {
      token: '{{member_name}}',
    })).toBe('e.g. {{member_name}} or fixed text')
  })
})
