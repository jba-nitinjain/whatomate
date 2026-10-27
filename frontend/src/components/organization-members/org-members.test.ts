import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en.json'
import es from '@/i18n/locales/es.json'
import pt from '@/i18n/locales/pt.json'
import ar from '@/i18n/locales/ar.json'
import hi from '@/i18n/locales/hi.json'
import ta from '@/i18n/locales/ta.json'
import type { Role } from '@/services/api'
import { navigationSections } from '@/components/layout/navigation'
import { pickDefaultRoleId } from './roles'

const locales: Record<string, any> = { en, es, pt, ar, hi, ta }

function role(partial: Partial<Role>): Role {
  return {
    id: 'r', name: 'custom', description: '', is_system: false, is_default: false,
    permissions: [], user_count: 0, created_at: '', updated_at: '', ...partial,
  }
}

describe('organisation members i18n', () => {
  it.each(Object.keys(locales))('%s has every orgMembers key and the nav label', (name) => {
    const block = locales[name].orgMembers
    expect(block).toBeDefined()
    for (const key of Object.keys(en.orgMembers)) {
      expect(typeof block[key], `${name}.orgMembers.${key}`).toBe('string')
    }
    expect(typeof locales[name].nav.orgMembers).toBe('string')
  })

  it.each(Object.keys(locales))('%s keeps the {name} placeholder in dialogs', (name) => {
    expect(locales[name].orgMembers.removeDesc).toContain('{name}')
    expect(locales[name].orgMembers.changeRoleDesc).toContain('{name}')
  })
})

describe('organisation members navigation', () => {
  it('lists the page under Settings gated by the organizations permission', () => {
    const settings = navigationSections.flatMap(s => s.items).find(i => i.path === '/settings')
    const item = settings?.children?.find(c => c.path === '/settings/organization-members')
    expect(item?.permission).toBe('organizations')
    expect(settings?.childPermissions).toContain('organizations')
  })
})

describe('pickDefaultRoleId', () => {
  it('prefers the organisation default role', () => {
    const roles = [role({ id: 'a', name: 'admin', is_system: true }), role({ id: 'd', is_default: true })]
    expect(pickDefaultRoleId(roles)).toBe('d')
  })

  it('falls back to the system agent role, then the first role', () => {
    expect(pickDefaultRoleId([role({ id: 'a', name: 'admin', is_system: true }), role({ id: 'g', name: 'agent', is_system: true })])).toBe('g')
    expect(pickDefaultRoleId([role({ id: 'x' })])).toBe('x')
    expect(pickDefaultRoleId([])).toBe('')
  })
})
