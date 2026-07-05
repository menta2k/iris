import { http } from './http'
import type {
  AppliedKumoConfig,
  KumoConfigApplyRequest,
  KumoConfigApplyResponse,
  KumoConfigPreview,
  KumoConfigStatus,
} from '@/types'

export const kumoConfigService = {
  /** Generate and preview the KumoMTA Lua policy without writing it. */
  generate() {
    return http.get<KumoConfigPreview>('/kumomta/config:generate')
  },
  /** Return the policy currently running (last applied) for diffing. */
  applied() {
    return http.get<AppliedKumoConfig>('/kumomta/config:applied')
  },
  /** Write the generated config to KumoMTA and reload the service. */
  apply(body: KumoConfigApplyRequest) {
    return http.post<KumoConfigApplyResponse>('/kumomta/config:apply', body)
  },
  /** Report whether the current config has drifted from the last applied policy. */
  status() {
    return http.get<KumoConfigStatus>('/kumomta/config:status')
  },
}
