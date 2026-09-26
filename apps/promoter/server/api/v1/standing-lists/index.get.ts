import { standingLists } from '../-mockDb'
export default defineEventHandler(() => [...standingLists].sort((a, b) => a.position - b.position))
