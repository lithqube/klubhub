import type { PostStatus } from '~/types/social'

/** Pure helper: maps post status to Tailwind dot color class. */
export function dotColorForStatus(status: PostStatus): string {
  switch (status) {
    case 'failed':
    case 'permanently_failed':
      return 'bg-secondary'
    case 'published':
    case 'publishing':
      return 'bg-on-surface-dim'
    case 'scheduled':
    case 'draft':
    default:
      return 'bg-primary'
  }
}
