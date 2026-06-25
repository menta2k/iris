import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type { ListResponse, WorkerErrorLog, WorkerErrorLogFilters } from '@/types'

export const workerErrorsService = {
  listWorkerErrorLogs(filters?: WorkerErrorLogFilters, page?: PageParams) {
    return http.get<ListResponse<WorkerErrorLog>>('/worker-error-logs', {
      query: pageQuery(page, { ...filters }),
    })
  },
}
