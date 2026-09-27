import { deviceList } from '../../-mockDb'
import { deviceSealedFields } from '../../-mockSealed'
// P2.6: each device also carries public_key and has_wrap (active version), as the API.
export default defineEventHandler(event => deviceList().map(d => ({ ...d, ...deviceSealedFields(event, d.id) })))
