import { computed, onBeforeUnmount, ref } from 'vue'
import { api, ApiError, errorMessage } from './api'
import { generateMessages } from './stream'
import type { Campaign, Messages, Preview, SavedMessages } from './types'

const copy = (messages: Messages | null): Messages => JSON.parse(JSON.stringify(messages ?? {})) as Messages
export function useCampaign(initial: Campaign) {
  const campaign = ref(initial), saved = ref<SavedMessages | null>(null), messages = ref<Messages>({})
  const audience = ref<Preview | null>(null), review = ref(false), error = ref(''), notice = ref('')
  const busy = ref(''), stale = ref(false), candidateRevision = ref<number | null>(null)
  const streamPreview = ref<Messages>({}), attempt = ref(0), generated = ref(false)
  let controller: AbortController | null = null, disposed = false
  const editable = computed(() => ['draft', 'content_ready'].includes(campaign.value.status))
  const dirty = computed(() => JSON.stringify(messages.value) !== JSON.stringify(saved.value?.messages ?? {}))
  const canSave = computed(() => editable.value && !busy.value && !stale.value && candidateRevision.value !== null && campaign.value.channels.every(ch => Boolean(messages.value[ch]?.body.trim())))
  const canApprove = computed(() => !busy.value && !stale.value && !dirty.value && !generated.value && review.value && campaign.value.status === 'content_ready' && saved.value?.validation?.revision === campaign.value.revision && audience.value?.revision === campaign.value.revision && Boolean(audience.value.approval_fingerprint) && audience.value.eligible_unique > 0)
  function invalidateReview() { review.value = false; audience.value = null }
  async function load(signal: AbortSignal, replace: boolean) {
    // Reads are individually coherent; never combine templates from another
    // revision with the campaign being reviewed.
    const c = await api.get(initial.id, signal), m = await api.messages(initial.id, signal)
    if (c.revision !== m.revision) throw new ApiError(409, 'concurrent_update')
    if (disposed) return
    campaign.value = c; saved.value = m
    if (replace) { messages.value = copy(m.messages); candidateRevision.value = c.revision; generated.value = false; stale.value = false; streamPreview.value = {} }
  }
  async function run(label: string, action: (signal: AbortSignal) => Promise<void>) {
    if (busy.value) return
    const current = new AbortController(); controller = current; busy.value = label; error.value = ''; notice.value = ''
    const signal = AbortSignal.any([current.signal, AbortSignal.timeout(40000)])
    try { await action(signal) }
    catch (e) {
      if (disposed) return
      error.value = errorMessage(e)
      if (e instanceof ApiError && [409, 412].includes(e.status)) {
        stale.value = true; invalidateReview()
        try { await load(signal, false) } catch { error.value += ' Refresh failed; retry refreshing before continuing.' }
      }
    } finally { if (controller === current) { busy.value = ''; controller = null } }
  }
  function refresh() { invalidateReview(); return run('Refreshing', signal => load(signal, true)) }
  function edit(value: Messages) { messages.value = value; invalidateReview() }
  function segment() {
    invalidateReview()
    return run('Generating segment', async signal => {
      campaign.value = await api.generateSegment(campaign.value, signal)
      await load(signal, true)
      notice.value = 'Segment generated. Review the audience, then generate or revalidate messages.'
    })
  }
  function preview() {
    invalidateReview()
    return run('Reviewing audience', async signal => {
      const p = await api.preview(initial.id, signal)
      if (p.revision !== campaign.value.revision) throw new ApiError(412, 'campaign_revision_conflict')
      audience.value = p
    })
  }
  function generate() {
    invalidateReview(); candidateRevision.value = null; generated.value = false; streamPreview.value = {}; attempt.value = 0
    return run('Generating messages', async signal => {
      const candidate = await generateMessages(initial.id, campaign.value.channels, signal, (kind, event) => {
        if (disposed) return
        if (kind === 'attempt') { attempt.value = event.attempt; streamPreview.value = {}; return }
        if (event.channel === 'sms') streamPreview.value.sms = { body: (streamPreview.value.sms?.body ?? '') + event.text }
        else if (event.channel === 'email') {
          const email = streamPreview.value.email ?? { subject: '', body: '' }
          email[event.field!] += event.text; streamPreview.value.email = email
        }
      })
      if (disposed) return
      messages.value = copy(candidate.messages); candidateRevision.value = candidate.base_revision; generated.value = true
      if (candidate.base_revision !== campaign.value.revision) throw new ApiError(412, 'campaign_revision_conflict')
      notice.value = 'Validated suggestion ready. Edit if needed, then explicitly save it.'
    })
  }
  function save() {
    if (!canSave.value || candidateRevision.value === null) return
    const revision = candidateRevision.value, snapshot = copy(messages.value)
    invalidateReview()
    return run('Saving messages', async signal => {
      const result = await api.save(initial.id, revision, snapshot, signal)
      saved.value = result; campaign.value = { ...campaign.value, revision: result.revision, status: result.status }
      messages.value = copy(result.messages); candidateRevision.value = result.revision; generated.value = false
      notice.value = 'Messages saved and validated. Review the current audience before approval.'
    })
  }
  function approve() {
    if (!canApprove.value || !audience.value) return
    const snapshot = audience.value
    review.value = false
    return run('Approving campaign', async signal => {
      const result = await api.approve(initial.id, snapshot, signal)
      campaign.value = { ...campaign.value, revision: result.revision, status: result.status }; audience.value = null
      notice.value = 'Campaign approved and queued for mock delivery.'
    })
  }
  onBeforeUnmount(() => { disposed = true; controller?.abort() })
  return { campaign, saved, messages, audience, review, error, notice, busy, stale, dirty, editable, canSave, canApprove, generated, streamPreview, attempt, refresh, edit, segment, preview, generate, save, approve, cancel: () => controller?.abort() }
}
