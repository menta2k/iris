import { http } from './http'
import type { UserDashboard } from '@/types'

interface ListResponse {
  dashboards?: UserDashboard[]
}

export interface CreateDashboardInput {
  name: string
  widgetsJson: string
  makeDefault?: boolean
}

export interface UpdateDashboardInput {
  name: string
  widgetsJson: string
}

// Per-user custom dashboards. Every call is implicitly scoped to the
// authenticated user by the backend; there is no user id in the path.
export const dashboardsService = {
  async list(): Promise<UserDashboard[]> {
    const res = await http.get<ListResponse>('/dashboards')
    return res.dashboards ?? []
  },
  create(input: CreateDashboardInput) {
    return http.post<UserDashboard>('/dashboards', {
      name: input.name,
      widgetsJson: input.widgetsJson,
      makeDefault: input.makeDefault ?? false,
    })
  },
  update(id: string, input: UpdateDashboardInput) {
    return http.put<UserDashboard>(`/dashboards/${encodeURIComponent(id)}`, {
      name: input.name,
      widgetsJson: input.widgetsJson,
    })
  },
  remove(id: string) {
    return http.delete<Record<string, never>>(`/dashboards/${encodeURIComponent(id)}`)
  },
  setDefault(id: string) {
    return http.post<UserDashboard>(`/dashboards/${encodeURIComponent(id)}:set-default`, {})
  },
}
