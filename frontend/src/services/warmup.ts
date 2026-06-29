import { http } from './http'
import type {
  CreateWarmupScheduleRequest,
  PauseWarmupScheduleRequest,
  UpdateWarmupScheduleRequest,
  WarmupListResponse,
  WarmupSchedule,
} from '@/types'

export const warmupService = {
  list(status?: string) {
    return http.get<WarmupListResponse>('/warmup-schedules', { query: { status } })
  },
  create(body: CreateWarmupScheduleRequest) {
    return http.post<WarmupSchedule>('/warmup-schedules', body)
  },
  update(id: string, body: UpdateWarmupScheduleRequest) {
    return http.put<WarmupSchedule>(`/warmup-schedules/${id}`, body)
  },
  pause(id: string, body: PauseWarmupScheduleRequest) {
    return http.post<WarmupSchedule>(`/warmup-schedules/${id}:pause`, body)
  },
  resume(id: string) {
    return http.post<WarmupSchedule>(`/warmup-schedules/${id}:resume`, {})
  },
}
