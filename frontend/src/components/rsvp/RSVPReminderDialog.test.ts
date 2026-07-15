// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RSVPReminderDialog from './RSVPReminderDialog.vue'

const api = vi.hoisted(() => ({
  get: vi.fn(),
  guests: vi.fn(),
  listReminders: vi.fn(),
  sendReminders: vi.fn(),
  templates: vi.fn(),
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, params?: Record<string, number>) => key === 'rsvp.recipientSelectionSummary'
    ? `${params?.selected} selected of ${params?.total} total`
    : key,
}) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/services/api', () => ({
  rsvpService: {
    get: api.get,
    guests: api.guests,
    listReminders: api.listReminders,
    sendReminders: api.sendReminders,
  },
  templatesService: { list: api.templates },
}))

describe('RSVPReminderDialog', () => {
  beforeEach(() => vi.clearAllMocks())

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
      global: { stubs: { Teleport: true } },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(document.body.textContent).toContain('rsvp.remindersTitle')
    expect(document.body.textContent).toContain('rsvp_message_1')
    expect(document.body.textContent).toContain('Member One')
    expect(wrapper.text()).toContain('1 selected of 1 total')

    const clearAll = wrapper.findAll('button').find(button => button.text() === 'rsvp.clearAllRecipients')
    expect(clearAll).toBeDefined()
    await clearAll!.trigger('click')
    expect(wrapper.text()).toContain('0 selected of 1 total')
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)

    const selectAll = wrapper.findAll('button').find(button => button.text() === 'common.selectAll')
    expect(selectAll).toBeDefined()
    await selectAll!.trigger('click')
    expect(wrapper.text()).toContain('1 selected of 1 total')
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.emitted('update:open')).toBeUndefined()
    wrapper.unmount()
  })

  it('shows the linked campaign after queueing all selected recipients', async () => {
    api.get.mockResolvedValue({ data: { data: {
      whatsapp_account: 'SBSM School',
      reminder_template_id: 'template-1',
    } } })
    api.listReminders.mockResolvedValue({ data: { data: { reminders: [] } } })
    api.templates.mockResolvedValue({ data: { data: { templates: [{
      id: 'template-1', name: 'rsvp_message_1', body_content: 'Please RSVP', buttons: [],
    }] } } })
    api.guests.mockResolvedValue({ data: { data: {
      total: 1,
      guests: [{ id: 'response-1', phone_number: '919999999999', contact: { profile_name: 'Member One' } }],
    } } })
    api.sendReminders.mockResolvedValue({ data: { data: {
      campaign_id: 'campaign-1',
      campaign_name: 'RSVP Reminder - Annual Gathering',
      queued: 1,
      skipped: 0,
    } } })

    const wrapper = mount(RSVPReminderDialog, {
      attachTo: document.body,
      props: { open: false, eventId: 'event-1', selectedIds: [] },
      global: { stubs: { Teleport: true } },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    const sendAll = wrapper.findAll('button').find(button => button.text().includes('rsvp.remindAllNotStarted'))
    expect(sendAll).toBeDefined()
    await sendAll!.trigger('click')
    await flushPromises()

    expect(api.sendReminders).toHaveBeenCalledWith('event-1', expect.objectContaining({
      all_not_started: true,
      template_id: 'template-1',
    }))
    expect(wrapper.text()).toContain('rsvp.reminderCampaignReady')
    expect(wrapper.text()).toContain('RSVP Reminder - Annual Gathering')
    expect(wrapper.emitted('changed')).toHaveLength(1)
    wrapper.unmount()
  })
})
