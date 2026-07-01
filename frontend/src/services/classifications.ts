import { http } from './http'
import type {
  SubjectClassification,
  CreateSubjectClassificationRequest,
  UpdateSubjectClassificationRequest,
} from '@/types'

interface ListReply {
  items?: SubjectClassification[]
}

export const classificationsService = {
  list() {
    return http.get<ListReply>('/subject-classifications')
  },
  create(body: CreateSubjectClassificationRequest) {
    return http.post<SubjectClassification>('/subject-classifications', body)
  },
  update(id: string, body: UpdateSubjectClassificationRequest) {
    return http.put<SubjectClassification>(
      `/subject-classifications/${encodeURIComponent(id)}`,
      body,
    )
  },
  remove(id: string) {
    return http.delete<{ ok: boolean }>(`/subject-classifications/${encodeURIComponent(id)}`)
  },
}
