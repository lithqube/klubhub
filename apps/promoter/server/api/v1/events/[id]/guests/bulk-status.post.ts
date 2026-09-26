import type { BulkStatusInput } from '~/types/guest'
import { bulkStatus } from '../../../-mockDb'
export default defineEventHandler(async event => bulkStatus(getRouterParam(event, 'id')!, await readBody<BulkStatusInput>(event)))
