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
    api.followUpPreview.mockResolvedValue({ data: { data: { eligible: 271, skipped: [], recipients: [] } } })
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
})
