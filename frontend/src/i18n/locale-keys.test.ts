import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en.json'
import es from '@/i18n/locales/es.json'
import pt from '@/i18n/locales/pt.json'
import ar from '@/i18n/locales/ar.json'
import hi from '@/i18n/locales/hi.json'
import ta from '@/i18n/locales/ta.json'

const locales: Record<string, any> = { en, es, pt, ar, hi, ta }

describe('locale keys used by components', () => {
  it.each(Object.keys(locales))('%s has the outgoing call error keys', (name) => {
    const block = locales[name].outgoingCalls
    expect(block).toBeDefined()
    expect(typeof block.callFailed).toBe('string')
    expect(typeof block.noPermission).toBe('string')
  })

  it.each(Object.keys(locales))('%s has the chatbot opt-in setting labels', (name) => {
    const block = locales[name].chatbotSettings
    for (const key of [
      'markBotHandledAsRead',
      'markBotHandledAsReadDesc',
      'skipTransferOutsideHours',
      'skipTransferOutsideHoursDesc',
    ]) {
      expect(typeof block[key], `${name}.chatbotSettings.${key}`).toBe('string')
    }
  })
})
