import { http } from './http'
import type {
  CreateEventProcessorRequest,
  EventProcessor,
  ListResponse,
  TestEventProcessorResult,
  UpdateEventProcessorRequest,
} from '@/types'

export const eventProcessorsService = {
  list() {
    return http.get<ListResponse<EventProcessor>>('/event-processors')
  },
  create(body: CreateEventProcessorRequest) {
    return http.post<EventProcessor>('/event-processors', body)
  },
  update(id: string, body: UpdateEventProcessorRequest) {
    return http.put<EventProcessor>(`/event-processors/${id}`, body)
  },
  remove(id: string) {
    return http.delete<Record<string, never>>(`/event-processors/${id}`)
  },
  test(body: CreateEventProcessorRequest) {
    return http.post<TestEventProcessorResult>('/event-processors:test', body)
  },
}
