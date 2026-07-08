import { http } from './http'
import { pageQuery, type PageParams } from './pagination'
import type {
  Bounce,
  BounceFilters,
  DsnMessage,
  FeedbackReport,
  ListResponse,
  MailRecord,
  MailRecordFilters,
  NextDeliveryAttempt,
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
  nextDeliveryAttempt(messageId: string) {
    return http.get<NextDeliveryAttempt>(`/mail-records/${encodeURIComponent(messageId)}/next-attempt`)
  },
  listBounces(filters?: BounceFilters, page?: PageParams) {
    return http.get<ListResponse<Bounce>>('/bounces', { query: pageQuery(page, { ...filters }) })
  },
  // Raw DSN notifications archived for a recipient (behind a dsn-type bounce).
  listDsnMessages(recipient: string) {
    return http.get<ListResponse<DsnMessage>>('/dsn-messages', { query: { recipient } })
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
