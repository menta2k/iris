import { http } from './http'
import type {
  AcmeAccount,
  AcmeCertificate,
  AcmeDnsProvider,
  AcmeDnsProviderInfo,
  ListResponse,
  RequestAcmeCertificateRequest,
  SaveAcmeAccountRequest,
  SetAcmeDnsProviderRequest,
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
  // DNS-01 provider configuration.
  listDnsProviders() {
    return http.get<ListResponse<AcmeDnsProviderInfo>>('/acme/dns-providers')
  },
  getDnsProvider() {
    return http.get<AcmeDnsProvider>('/acme/dns-provider')
  },
  setDnsProvider(body: SetAcmeDnsProviderRequest) {
    return http.put<AcmeDnsProvider>('/acme/dns-provider', body)
  },
  clearDnsProvider() {
    return http.delete<AcmeDnsProvider>('/acme/dns-provider')
  },
}
