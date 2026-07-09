import { http } from './http'
import type {
  InjectionCredential,
  CreateInjectionCredentialRequest,
  UpdateInjectionCredentialRequest,
} from '@/types'

interface ListReply {
  items?: InjectionCredential[]
}

export const injectionCredentialsService = {
  list() {
    return http.get<ListReply>('/injection-credentials')
  },
  create(body: CreateInjectionCredentialRequest) {
    return http.post<InjectionCredential>('/injection-credentials', body)
  },
  update(id: string, body: UpdateInjectionCredentialRequest) {
    return http.put<InjectionCredential>(`/injection-credentials/${encodeURIComponent(id)}`, body)
  },
  setPassword(id: string, password: string) {
    return http.post<InjectionCredential>(
      `/injection-credentials/${encodeURIComponent(id)}/password`,
      { id, password },
    )
  },
  remove(id: string) {
    return http.delete<{ ok: boolean }>(`/injection-credentials/${encodeURIComponent(id)}`)
  },
}
