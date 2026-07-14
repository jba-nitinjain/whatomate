// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import RSVPReminderDialog from './RSVPReminderDialog.vue'

const api = vi.hoisted(() => ({
  get: vi.fn(),
  guests: vi.fn(),
  listReminders: vi.fn(),
  templates: vi.fn(),
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/services/api', () => ({
  rsvpService: {
    get: api.get,
    guests: api.guests,
    listReminders: api.listReminders,
  },
  templatesService: { list: api.templates },
}))

describe('RSVPReminderDialog', () => {
  it('renders loaded templates, schedules, and recipients without closing', async () => {
    api.get.mockResolvedValue({ data: { data: {
      whatsapp_account: 'SBSM School',
      reminder_template_id: 'template-1',
    } } })
    api.listReminders.mockResolvedValue({ data: { data: { reminders: [] } } })
    api.templates.mockResolvedValue({ data: { data: { templates: [{
      id: 'template-1',
      name: 'rsvp_message_1',
      body_content: 'Hello {{1}}, reminder for {{2}}',
      buttons: [],
    }] } } })
    api.guests.mockResolvedValue({ data: { data: {
      total: 1,
      guests: [{
        id: 'response-1',
        phone_number: '919999999999',
        contact: { profile_name: 'Member One' },
      }],
    } } })

    const wrapper = mount(RSVPReminderDialog, {
      attachTo: document.body,
      props: { open: false, eventId: 'event-1', selectedIds: [] },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(document.body.textContent).toContain('rsvp.remindersTitle')
    expect(document.body.textContent).toContain('rsvp_message_1')
    expect(document.body.textContent).toContain('Member One')
    expect(wrapper.emitted('update:open')).toBeUndefined()
    wrapper.unmount()
  })
})
