// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import RSVPFollowUpDialog from './RSVPFollowUpDialog.vue'

const api = vi.hoisted(() => ({
  get: vi.fn(),
  followUpPreview: vi.fn(),
  sendFollowUp: vi.fn(),
  uploadReminderMedia: vi.fn(),
  templates: vi.fn(),
  listFlows: vi.fn(),
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, params?: Record<string, any>) => {
    if (key === 'rsvp.followUpCount') return `${params?.count} guests will be asked`
    if (key === 'rsvp.followUpSend') return `Send follow-up (${params?.count})`
    if (key === 'rsvp.recipientSelectionSummary') return `${params?.selected} selected of ${params?.total} total`
    return key
  },
}) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/services/api', () => ({
  rsvpService: {
    get: api.get,
    followUpPreview: api.followUpPreview,
    sendFollowUp: api.sendFollowUp,
    uploadReminderMedia: api.uploadReminderMedia,
  },
  templatesService: { list: api.templates },
  chatbotService: { listFlows: api.listFlows },
}))

const PLAIN_TEMPLATE = { id: 'template-1', name: 'children_followup', body_content: 'How many children?', buttons: [] }

async function mountDialog() {
  const wrapper = mount(RSVPFollowUpDialog, {
    attachTo: document.body,
    props: { open: false, eventId: 'event-1' },
    global: { stubs: { Teleport: true } },
  })
  await wrapper.setProps({ open: true })
  await flushPromises()
  return wrapper
}

describe('RSVPFollowUpDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockResolvedValue({ data: { data: { whatsapp_account: 'SBSM School', flow_id: 'flow-primary' } } })
    api.templates.mockResolvedValue({ data: { data: { templates: [PLAIN_TEMPLATE] } } })
    api.listFlows.mockResolvedValue({ data: { data: { flows: [
      { id: 'flow-primary', name: 'RSVP intake', whatsapp_account: 'SBSM School' },
      { id: 'flow-children', name: 'Children count follow-up', whatsapp_account: 'SBSM School' },
    ] } } })
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 271, skipped: [], recipients: [
      { id: 'response-1', phone_number: '919999999999', contact: { profile_name: 'Member One' } },
    ] } } })
  })

  it('excludes the event primary flow from the flow picker', async () => {
    const wrapper = await mountDialog()

    const options = wrapper.findAll('option').map(o => o.text())
    expect(options).toContain('Children count follow-up')
    expect(options).not.toContain('RSVP intake')
    wrapper.unmount()
  })

  it('blocks send with the missing-answer-key reason by default, without calling preview', async () => {
    const wrapper = await mountDialog()

    // Default audience is missing_answer with no key entered yet.
    expect(api.followUpPreview).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('rsvp.followUpAnswerKeyRequired')
    const sendButton = wrapper.findAll('button').find(b => b.text().includes('Send follow-up'))
    expect(sendButton).toBeDefined()
    expect(sendButton!.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('loads the live audience count and the shrink explanation when the audience changes', async () => {
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    await flushPromises()

    expect(api.followUpPreview).toHaveBeenCalledWith('event-1', 'not_started', undefined)
    expect(wrapper.text()).toContain('271 guests will be asked')
    expect(wrapper.text()).toContain('rsvp.followUpCountShrinks')
    wrapper.unmount()
  })

  it('blocks send until a template and flow are chosen, then unblocks', async () => {
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    await flushPromises()

    let sendButton = wrapper.findAll('button').find(b => b.text().includes('Send follow-up'))
    expect(sendButton!.attributes('disabled')).toBeDefined()

    vm.templateId = 'template-1'
    vm.onTemplateChange()
    vm.flowId = 'flow-children'
    await flushPromises()

    sendButton = wrapper.findAll('button').find(b => b.text().includes('Send follow-up'))
    expect(sendButton!.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('shows the linked campaign after a successful send and refreshes the shrinking count', async () => {
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    vm.templateId = 'template-1'
    vm.onTemplateChange()
    vm.flowId = 'flow-children'
    await flushPromises()

    api.sendFollowUp.mockResolvedValue({ data: { data: {
      requested: 271, queued: 271, skipped: [], campaign_id: 'campaign-1', campaign_name: 'RSVP Follow-up - Annual Gathering',
    } } })
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 0, skipped: [], recipients: [] } } })

    await vm.send()
    await flushPromises()

    expect(api.sendFollowUp).toHaveBeenCalledWith('event-1', expect.objectContaining({
      audience: 'not_started',
      template_id: 'template-1',
      flow_id: 'flow-children',
    }))
    expect(wrapper.text()).toContain('rsvp.followUpCampaignReady')
    expect(wrapper.text()).toContain('RSVP Follow-up - Annual Gathering')
    expect(wrapper.emitted('changed')).toHaveLength(1)
    wrapper.unmount()
  })

  it('reports a run where every recipient failed as an error, not a success', async () => {
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    vm.templateId = 'template-1'
    vm.onTemplateChange()
    vm.flowId = 'flow-children'
    await flushPromises()

    // The real endpoint does not currently return sent/failed at all (only
    // requested/queued/skipped), so this is defensive - it mirrors
    // RSVPReminderDialog's rule for the shape the response would need to have
    // for this branch to matter.
    api.sendFollowUp.mockResolvedValue({ data: { data: {
      queued: 271, sent: 0, failed: 271, skipped: [],
      recipients: [{ error_message: 'follow-up flow was not found' }],
    } } })

    await vm.send()
    await flushPromises()

    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('follow-up flow was not found'))
    expect(toast.success).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('rsvp.followUpCampaignReady')
    wrapper.unmount()
  })

  const TWO_RECIPIENTS = [
    { id: 'response-1', phone_number: '919999999999', contact: { profile_name: 'Member One' } },
    { id: 'response-2', phone_number: '918888888888', contact: { profile_name: 'Member Two' } },
  ]

  it('renders the recipient list selected by default, and lets the admin narrow the selection', async () => {
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 2, skipped: [], recipients: TWO_RECIPIENTS } } })
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    await flushPromises()

    expect(wrapper.text()).toContain('Member One')
    expect(wrapper.text()).toContain('Member Two')
    expect(wrapper.text()).toContain('2 selected of 2 total')
    let sendButton = wrapper.findAll('button').find(b => b.text().includes('Send follow-up'))
    expect(sendButton!.text()).toContain('Send follow-up (2)')

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(2)
    await checkboxes[0].trigger('change')

    expect(wrapper.text()).toContain('1 selected of 2 total')
    sendButton = wrapper.findAll('button').find(b => b.text().includes('Send follow-up'))
    expect(sendButton!.text()).toContain('Send follow-up (1)')
    wrapper.unmount()
  })

  it('clear all then select all restore the full default selection, matching RSVPReminderDialog', async () => {
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 2, skipped: [], recipients: TWO_RECIPIENTS } } })
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    await flushPromises()

    const clearAll = wrapper.findAll('button').find(b => b.text() === 'rsvp.clearAllRecipients')
    expect(clearAll).toBeDefined()
    await clearAll!.trigger('click')
    expect(wrapper.text()).toContain('0 selected of 2 total')
    for (const checkbox of wrapper.findAll('input[type="checkbox"]')) {
      expect((checkbox.element as HTMLInputElement).checked).toBe(false)
    }

    const selectAll = wrapper.findAll('button').find(b => b.text() === 'common.selectAll')
    expect(selectAll).toBeDefined()
    await selectAll!.trigger('click')
    expect(wrapper.text()).toContain('2 selected of 2 total')
    for (const checkbox of wrapper.findAll('input[type="checkbox"]')) {
      expect((checkbox.element as HTMLInputElement).checked).toBe(true)
    }
    wrapper.unmount()
  })

  it('filters the recipient list by name or phone number without changing the total selected', async () => {
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 2, skipped: [], recipients: TWO_RECIPIENTS } } })
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    await flushPromises()

    vm.recipientSearch = 'Member Two'
    vm.onRecipientSearchInput()
    await flushPromises()

    expect(wrapper.text()).not.toContain('Member One')
    expect(wrapper.text()).toContain('Member Two')
    // Search narrows what's shown, not the underlying selection - both
    // recipients (including the filtered-out one) stay selected, mirroring
    // RSVPReminderDialog.vue where "select all" targets the whole audience.
    expect(wrapper.text()).toContain('2 selected of 2 total')
    wrapper.unmount()
  })

  it('omits response_ids for the untouched default selection, but sends only the chosen ids once narrowed', async () => {
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 2, skipped: [], recipients: TWO_RECIPIENTS } } })
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    vm.templateId = 'template-1'
    vm.onTemplateChange()
    vm.flowId = 'flow-children'
    await flushPromises()

    api.sendFollowUp.mockResolvedValue({ data: { data: { queued: 2, skipped: [] } } })
    await vm.send()
    await flushPromises()

    // Every recipient selected (the default) is the same set the server
    // would pick from audience/answer_key alone, so response_ids is left off
    // entirely - it stays self-cleaning as guests answer between preview and
    // send, which is the pre-existing default behaviour this task preserves.
    let sentPayload = api.sendFollowUp.mock.calls.at(-1)![1]
    expect(sentPayload.response_ids).toBeUndefined()

    // Narrow the selection to just one recipient and send again.
    vm.toggleRecipient('response-1')
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 1, skipped: [], recipients: [TWO_RECIPIENTS[1]] } } })
    await vm.send()
    await flushPromises()

    sentPayload = api.sendFollowUp.mock.calls.at(-1)![1]
    expect(sentPayload.response_ids).toEqual(['response-2'])
    wrapper.unmount()
  })

  it('resets the recipient selection when the audience changes, rather than carrying a stale selection into the new list', async () => {
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 2, skipped: [], recipients: TWO_RECIPIENTS } } })
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    await flushPromises()
    vm.toggleRecipient('response-1')
    expect(vm.selectedRecipientCount).toBe(1)

    const OTHER_RECIPIENT = { id: 'response-3', phone_number: '917777777777', contact: { profile_name: 'Member Three' } }
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 1, skipped: [], recipients: [OTHER_RECIPIENT] } } })
    vm.audience = 'responded_yes'
    vm.onAudienceChange()
    await flushPromises()

    // The new audience's one guest must be selected by default - a stale
    // exclusion from the previous audience must not leak forward.
    expect(vm.selectedRecipientCount).toBe(1)
    expect(vm.isRecipientSelected('response-3')).toBe(true)
    wrapper.unmount()
  })

  it('blocks send when every recipient has been deselected', async () => {
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 2, skipped: [], recipients: TWO_RECIPIENTS } } })
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.audience = 'not_started'
    vm.onAudienceChange()
    vm.templateId = 'template-1'
    vm.onTemplateChange()
    vm.flowId = 'flow-children'
    await flushPromises()

    let sendButton = wrapper.findAll('button').find(b => b.text().includes('Send follow-up'))
    expect(sendButton!.attributes('disabled')).toBeUndefined()

    const clearAll = wrapper.findAll('button').find(b => b.text() === 'rsvp.clearAllRecipients')
    await clearAll!.trigger('click')

    sendButton = wrapper.findAll('button').find(b => b.text().includes('Send follow-up'))
    expect(sendButton!.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
