import { http } from './http'
import type {
  MonitoringAccount,
  MonitoringProbe,
  MonitoringProbeRaw,
  ProbeEvent,
  CreateMonitoringAccountRequest,
  UpdateMonitoringAccountRequest,
  VerifyMonitoringAccountRequest,
  VerifyMonitoringAccountReply,
} from '@/types'

interface AccountsReply {
  items?: MonitoringAccount[]
}

interface ProbesReply {
  items?: MonitoringProbe[]
  nextPageToken?: string
}

const base = '/monitoring/accounts'

export const monitoringService = {
  listAccounts() {
    return http.get<AccountsReply>(base)
  },
  createAccount(body: CreateMonitoringAccountRequest) {
    return http.post<MonitoringAccount>(base, body)
  },
  updateAccount(id: string, body: UpdateMonitoringAccountRequest) {
    return http.put<MonitoringAccount>(`${base}/${encodeURIComponent(id)}`, body)
  },
  setPassword(id: string, password: string) {
    return http.post<MonitoringAccount>(`${base}/${encodeURIComponent(id)}/password`, { id, password })
  },
  removeAccount(id: string) {
    return http.delete<{ ok: boolean }>(`${base}/${encodeURIComponent(id)}`)
  },
  sendProbe(accountId: string) {
    return http.post<MonitoringProbe>(`${base}/${encodeURIComponent(accountId)}/probe`, { accountId })
  },
  verify(body: VerifyMonitoringAccountRequest) {
    return http.post<VerifyMonitoringAccountReply>(`${base}:verify`, body)
  },
  listProbes(accountId: string, params?: { pageSize?: number; pageToken?: string }) {
    return http.get<ProbesReply>(`${base}/${encodeURIComponent(accountId)}/probes`, {
      query: { page_size: params?.pageSize, page_token: params?.pageToken },
    })
  },
  probeRaw(probeId: string) {
    return http.get<MonitoringProbeRaw>(`/monitoring/probes/${encodeURIComponent(probeId)}/raw`)
  },
  probeEvents(probeId: string) {
    return http.get<{ items?: ProbeEvent[] }>(`/monitoring/probes/${encodeURIComponent(probeId)}/events`)
  },
}
