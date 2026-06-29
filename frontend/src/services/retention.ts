import { http } from './http'
import type { RetentionPolicy, RetentionView, UpdateRetentionPolicyRequest } from '@/types'

export const retentionService = {
  listRetention() {
    return http.get<{ items: RetentionView[] }>('/retention')
  },
  updateRetention(table: string, body: UpdateRetentionPolicyRequest) {
    return http.put<RetentionPolicy>(`/retention/${table}`, body)
  },
  runRetention(tableName: string) {
    return http.post<{ ok: boolean }>('/retention:run', { table_name: tableName })
  },
}
