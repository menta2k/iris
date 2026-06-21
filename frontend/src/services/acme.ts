import { http } from './http'
import type {
  AcmeAccount,
  AcmeCertificate,
  ListResponse,
  RequestAcmeCertificateRequest,
  SaveAcmeAccountRequest,
} from '@/types'

export const acmeService = {
  getAccount() {
    return http.get<AcmeAccount>('/acme/account')
  },
  saveAccount(body: SaveAcmeAccountRequest) {
    return http.put<AcmeAccount>('/acme/account', body)
  },
  listCertificates() {
    return http.get<ListResponse<AcmeCertificate>>('/acme/certificates')
  },
  requestCertificate(body: RequestAcmeCertificateRequest) {
    return http.post<AcmeCertificate>('/acme/certificates', body)
  },
  deleteCertificate(id: string) {
    return http.delete<Record<string, never>>(`/acme/certificates/${id}`)
  },
}
