import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import RSVPFollowUpDialog from './RSVPFollowUpDialog.vue'
import { rsvpService, templatesService, chatbotService } from '@/services/api'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() })
}))

vi.mock('vue-sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() }
}))

vi.mock('@/services/api', () => ({
  rsvpService: {
    get: vi.fn(),
    followUpPreview: vi.fn(),
    sendFollowUp: vi.fn(),
    uploadReminderMedia: vi.fn(),
  },
  templatesService: {
    list: vi.fn()
  },
  chatbotService: {
    listFlows: vi.fn()
  }
}))

const VIDEO_TEMPLATE = { id: 'tpl-video', name: 'Video follow-up', header_type: 'VIDEO', body_content: 'Hi {{member_name}}', buttons: [] }
const IMAGE_TEMPLATE = { id: 'tpl-image', name: 'Image follow-up', header_type: 'IMAGE', body_content: 'Hi {{member_name}}', buttons: [] }

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} } })

function mockLoadResponses() {
  ;(rsvpService.get as any).mockResolvedValue({ data: { data: { whatsapp_account: 'acct-1', flow_id: 'flow-primary' } } })
  ;(templatesService.list as any).mockResolvedValue({ data: { data: { templates: [VIDEO_TEMPLATE, IMAGE_TEMPLATE] } } })
  ;(chatbotService.listFlows as any).mockResolvedValue({ data: { data: { flows: [{ id: 'flow-other', name: 'Children flow', whatsapp_account: 'acct-1' }] } } })
  ;(rsvpService.followUpPreview as any).mockResolvedValue({ data: { data: { eligible: 0, skipped: [], recipients: [] } } })
}

async function mountDialog() {
  const wrapper = mount(RSVPFollowUpDialog, {
    props: { open: false, eventId: 'event-1' },
    global: { plugins: [i18n] }
  })
  // The dialog only loads data when `open` transitions to true (the
  // props.open watcher has no `immediate: true`), matching how the real
  // parent component toggles visibility.
  await wrapper.setProps({ open: true })
  await flushPromises()
  return wrapper
}

function makeFileChangeEvent(file: File): Event {
  return { target: { files: [file], value: '' } } as unknown as Event
}

describe('RSVPFollowUpDialog media upload/template-switch race', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockLoadResponses()
  })

  it('discards a media upload response for a template that was superseded before it resolved', async () => {
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.templateId = 'tpl-video'
    await flushPromises()

    let resolveUpload!: (value: unknown) => void
    ;(rsvpService.uploadReminderMedia as any).mockImplementation(
      () => new Promise(resolve => { resolveUpload = resolve })
    )

    const file = new File(['fake video bytes'], 'clip.mp4', { type: 'video/mp4' })
    const uploadPromise = vm.onMediaFileChange(makeFileChangeEvent(file))

    // While the tpl-video upload is still in flight, the user switches to the
    // IMAGE template. onTemplateChange clears stagedMedia immediately; without
    // the fix in onMediaFileChange, the tpl-video upload resolving afterwards
    // would silently repopulate stagedMedia with a file staged for the
    // template the user is no longer on.
    vm.templateId = 'tpl-image'
    vm.onTemplateChange()
    await flushPromises()

    resolveUpload({ data: { data: { staging_id: 'staging-video', filename: 'clip.mp4', mime_type: 'video/mp4' } } })
    await uploadPromise
    await flushPromises()

    expect(vm.stagedMedia).toBeNull()
  })

  it('keeps a media upload response when the template was not changed while it was in flight', async () => {
    const wrapper = await mountDialog()
    const vm = wrapper.vm as any

    vm.templateId = 'tpl-video'
    await flushPromises()

    ;(rsvpService.uploadReminderMedia as any).mockResolvedValue({
      data: { data: { staging_id: 'staging-video', filename: 'clip.mp4', mime_type: 'video/mp4' } }
    })

    const file = new File(['fake video bytes'], 'clip.mp4', { type: 'video/mp4' })
    await vm.onMediaFileChange(makeFileChangeEvent(file))
    await flushPromises()

    expect(vm.stagedMedia).toEqual({
      staging_id: 'staging-video',
      filename: 'clip.mp4',
      mime_type: 'video/mp4'
    })
  })
})
