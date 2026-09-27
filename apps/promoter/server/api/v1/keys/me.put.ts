import { putMemberKey } from '../-mockSealed'
// PUT /api/v1/keys/me (account.self): create the member key (201) or re-wrap its private key (200);
// a different public key needs security.manage (recovery), else 409 public_key_mismatch.
export default defineEventHandler(async (event) => {
  const r = putMemberKey(event, await readBody(event))
  setResponseStatus(event, r.created ? 201 : 200)
  return r.key
})
