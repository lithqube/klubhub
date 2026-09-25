import { checkToken, fail, findInvoice, issue, issueProblems, serialize } from '../../-mockDb'

export default defineEventHandler(async (event) => {
  const inv = findInvoice(event)
  if (!inv) return fail(event, 404, 'not_found', 'invoice not found')
  const body = ((await readBody(event).catch(() => null)) ?? {}) as Record<string, unknown>
  const tokenErr = checkToken(event, inv, body.updated_at)
  if (tokenErr) return tokenErr
  if (inv.status !== 'draft') return fail(event, 409, 'bad_state', 'only drafts can be issued')
  const problems = issueProblems(inv)
  if (problems.length) return fail(event, 422, 'not_issuable', 'invoice is not ready to issue', problems)
  issue(inv)
  return { data: serialize(inv) }
})
