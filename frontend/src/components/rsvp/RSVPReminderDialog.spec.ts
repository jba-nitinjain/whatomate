import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import RSVPReminderDialog from './RSVPReminderDialog.vue'
import { rsvpService, templatesService } from '@/services/api'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() })
}))

vi.mock('vue-sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() }
}))

vi.mock('@/services/api', () => ({
  rsvpService: {
    get: vi.fn(),
    listReminders: vi.fn(),
    guests: vi.fn(),
    reminderPreview: vi.fn(),
    uploadReminderMedia: vi.fn(),
    sendReminders: vi.fn(),
    createReminder: vi.fn(),
    cancelReminder: vi.fn()
  },
  templatesService: {
    list: vi.fn()
  }
}))

const VIDEO_TEMPLATE = { id: 'tpl-video', name: 'Video reminder', header_type: 'VIDEO', body_content: 'Hi {{member_name}}', buttons: [] }
const IMAGE_TEMPLATE = { id: 'tpl-image', name: 'Image reminder', header_type: 'IMAGE', body_content: 'Hi {{member_name}}', buttons: [] }

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} } })

function mockLoadResponses() {
  ;(rsvpService.get as any).mockResolvedValue({ data: { data: { whatsapp_account: 'acct-1', reminder_template_id: '' } } })
  ;(rsvpService.listReminders as any).mockResolvedValue({ data: { data: { reminders: [] } } })
  ;(templatesService.list as any).mockResolvedValue({ data: { data: { templates: [VIDEO_TEMPLATE, IMAGE_TEMPLATE] } } })
  ;(rsvpService.guests as any).mockResolvedValue({ data: { data: { guests: [], total: 0 } } })
  ;(rsvpService.reminderPreview as any).mockResolvedValue({ data: { data: { skipped: [] } } })
}

async function mountDialog() {
  const wrapper = mount(RSVPReminderDialog, {
    props: { open: false, eventId: 'event-1', selectedIds: [] },
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

describe('RSVPReminderDialog media upload/template-switch race', () => {
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
    // IMAGE template. The templateId watcher clears stagedMedia immediately;
    // without the fix in onMediaFileChange, the tpl-video upload resolving
    // afterwards would silently repopulate stagedMedia with a file staged for
    // the template the user is no longer on.
    vm.templateId = 'tpl-image'
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
