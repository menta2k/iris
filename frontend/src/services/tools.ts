import { http } from './http'
import type { DiagnoseRequest, DiagnoseResult, RblCheckReply } from '@/types'

export const toolsService = {
  diagnose(body: DiagnoseRequest) {
    return http.post<DiagnoseResult>('/tools/diagnose', body)
  },
  rblCheck() {
    return http.post<RblCheckReply>('/tools/rbl-check', {})
  },
}
