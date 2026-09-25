import { fail, findInvoice, issueProblems } from '../../-mockDb'

export default defineEventHandler((event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  if (inv.status !== 'draft') return { data: { ready: false, problems: [{ field: 'status', message: 'Only drafts can be issued.' }] } }
  const problems = issueProblems(inv)
  return { data: { ready: problems.length === 0, problems } }
})
