import { http } from './http'
import type { MTANode, CreateMTANodeRequest, UpdateMTANodeRequest } from '@/types'

interface ListReply {
  items?: MTANode[]
}

export const clusterService = {
  listNodes() {
    return http.get<ListReply>('/cluster/nodes')
  },
  getNode(id: string) {
    return http.get<MTANode>(`/cluster/nodes/${encodeURIComponent(id)}`)
  },
  createNode(body: CreateMTANodeRequest) {
    return http.post<MTANode>('/cluster/nodes', body)
  },
  updateNode(id: string, body: UpdateMTANodeRequest) {
    return http.put<MTANode>(`/cluster/nodes/${encodeURIComponent(id)}`, body)
  },
  removeNode(id: string) {
    return http.delete<{ ok: boolean }>(`/cluster/nodes/${encodeURIComponent(id)}`)
  },
}
