import type { PostStatus } from '~/types/social'

/** Pure helper: maps post status to Tailwind dot color class. */
export function dotColorForStatus(status: PostStatus): string {
  switch (status) {
    case 'failed':
    case 'permanently_failed':
      return 'bg-error'
    case 'published':
    case 'publishing':
      return 'bg-tertiary'
    case 'draft':
      return 'bg-secondary'
    case 'scheduled':
    default:
      return 'bg-primary'
  }
}
