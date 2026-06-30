import { http } from './http'
import type {
  CreateDeliveryBlueprintRequest,
  DeliveryBlueprint,
  ListResponse,
  SeedDeliveryBlueprintsResponse,
  UpdateDeliveryBlueprintRequest,
} from '@/types'

export const blueprintsService = {
  list() {
    return http.get<ListResponse<DeliveryBlueprint>>('/delivery-blueprints')
  },
  create(body: CreateDeliveryBlueprintRequest) {
    return http.post<DeliveryBlueprint>('/delivery-blueprints', body)
  },
  update(id: string, body: UpdateDeliveryBlueprintRequest) {
    return http.put<DeliveryBlueprint>(`/delivery-blueprints/${id}`, body)
  },
  setStatus(id: string, status: 'active' | 'disabled') {
    return http.post<DeliveryBlueprint>(`/delivery-blueprints/${id}:status`, { status })
  },
  seedDefaults() {
    return http.post<SeedDeliveryBlueprintsResponse>('/delivery-blueprints:seed-defaults', {})
  },
}
