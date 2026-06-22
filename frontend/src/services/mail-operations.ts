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
  listMailRecords(
    filters?: MailRecordFilters,
    page?: { pageSize?: number; pageToken?: string },
  ) {
    // The nested PageRequest binds via dot notation; the form codec accepts the
    // proto field names (page.page_size / page.page_token).
    const query: Record<string, string | number | undefined> = { ...filters }
    if (page?.pageSize) query['page.page_size'] = page.pageSize
    if (page?.pageToken) query['page.page_token'] = page.pageToken
    return http.get<ListResponse<MailRecord>>('/mail-records', { query })
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
