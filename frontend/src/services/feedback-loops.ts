import { http } from './http'
import type {
  CreateFeedbackLoopRequest,
  FeedbackLoop,
  ListResponse,
  UpdateFeedbackLoopRequest,
} from '@/types'

export const feedbackLoopsService = {
  listFeedbackLoops() {
    return http.get<ListResponse<FeedbackLoop>>('/feedback-loops')
  },
  createFeedbackLoop(body: CreateFeedbackLoopRequest) {
    return http.post<FeedbackLoop>('/feedback-loops', body)
  },
  updateFeedbackLoop(id: string, body: UpdateFeedbackLoopRequest) {
    return http.put<FeedbackLoop>(`/feedback-loops/${id}`, body)
  },
  deleteFeedbackLoop(id: string) {
    return http.delete<void>(`/feedback-loops/${id}`)
  },
}
