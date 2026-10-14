export type PostStatus = 'draft' | 'scheduled' | 'publishing' | 'published' | 'failed' | 'permanently_failed'
export type PostType = 'feed' | 'story'

export interface SocialAccount {
  id: string
  platform: 'instagram'
  igUserId: string
  accountName: string
  tokenExpiry: string | null  // ISO 8601
  status: 'connected' | 'disconnected'
  createdAt: string
  updatedAt: string
}

export interface ScheduledPost {
  id: string
  accountId: string
  status: PostStatus
  postType: PostType
  caption: string
  imageStorageKey: string  // Garage object key for the post image (canonical; legacy name was imageMinioPath)
  scheduledAtUtc: string  // ISO 8601
  timezoneName: string
  retryCount: number
  nextRetryAt: string | null
  lastError: string | null
  containerId: string | null
  createdAt: string
  updatedAt: string
}

export interface CreatePostRequest {
  postType: PostType
  caption: string
  scheduledAt: string  // datetime-local format: "2026-10-24T23:45"
  timezoneName: string
  imageId?: string     // existing Garage object key (canonical name — carries a Garage object key, not a separate API field)
  accountId?: string   // optional account ID to override the current account
}

export interface EditPostRequest {
  caption?: string
  scheduledAt?: string
  timezoneName?: string
  imageId?: string
}
