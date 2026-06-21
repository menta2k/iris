import { http } from './http'
import type { GlobalSettings, UpdateGlobalSettingsRequest } from '@/types'

export const settingsService = {
  getSettings() {
    return http.get<GlobalSettings>('/settings')
  },
  updateSettings(body: UpdateGlobalSettingsRequest) {
    return http.put<GlobalSettings>('/settings', body)
  },
}
