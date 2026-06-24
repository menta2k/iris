import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type {
  Bounce,
  FeedbackReport,
  ListResponse,
  MailRecord,
  MailRecordFilters,
  Queue,
  QueueActionRequest,
  QueueActionResponse,
  ServiceControlRequest,
  ServiceControlResponse,
} from '@/types'

export const mailOperationsService = {
  listMailRecords(filters?: MailRecordFilters, page?: PageParams) {
    return http.get<ListResponse<MailRecord>>('/mail-records', {
      query: pageQuery(page, { ...filters }),
    })
  },
  listBounces(page?: PageParams) {
    return http.get<ListResponse<Bounce>>('/bounces', { query: pageQuery(page) })
  },
  listFeedbackReports(page?: PageParams) {
    return http.get<ListResponse<FeedbackReport>>('/feedback-reports', { query: pageQuery(page) })
  },
  listQueues() {
    return http.get<ListResponse<Queue>>('/queues')
  },
  queueAction(body: QueueActionRequest) {
    return http.post<QueueActionResponse>('/queues:action', body)
  },
  serviceControl(body: ServiceControlRequest) {
    return http.post<ServiceControlResponse>('/kumomta:service-control', body)
  },
}
