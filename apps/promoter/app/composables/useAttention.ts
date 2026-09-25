import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import { useSessionStore } from '~/stores/session'
import { attentionQueue } from '~/utils/attention'

/** The dashboard queue, shared with the nav counter. */
export function useAttention() {
  const events = useEventStore()
  const { lists } = storeToRefs(events)
  const { needsMfa } = storeToRefs(useSessionStore())
  const items = computed(() => attentionQueue(lists.value.upcoming, lists.value.drafts, { needsMfa: needsMfa.value }))
  const hasConflict = computed(() => items.value.some(i => i.kind === 'conflict'))
  const load = () => Promise.all([events.fetchList('upcoming'), events.fetchList('drafts')])
  return { items, hasConflict, load }
}
