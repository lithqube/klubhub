import { mockOrg } from './-mockDb'

// Mock for frontend-only development (no NUXT_PUBLIC_API_BASE).
export default defineEventHandler(() => mockOrg())
