import { http } from './http'
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
  listMailRecords(filters?: MailRecordFilters) {
    return http.get<ListResponse<MailRecord>>('/mail-records', { query: filters })
  },
  listBounces() {
    return http.get<ListResponse<Bounce>>('/bounces')
  },
  listFeedbackReports() {
    return http.get<ListResponse<FeedbackReport>>('/feedback-reports')
  },
  listQueues() {
    return http.get<ListResponse<Queue>>('/queues')
  },
  queueAction(mailclass: string, body: QueueActionRequest) {
    return http.post<QueueActionResponse>(
      `/queues/${encodeURIComponent(mailclass)}:action`,
      body,
    )
  },
  serviceControl(body: ServiceControlRequest) {
    return http.post<ServiceControlResponse>('/kumomta:service-control', body)
  },
}
