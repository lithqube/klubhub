import { retentionPreview } from '../../-mockDb'

// Mirrors GET /api/v1/org/retention/preview?days=N: {would_purge, count}.
export default defineEventHandler(event => retentionPreview(getQuery(event)))
