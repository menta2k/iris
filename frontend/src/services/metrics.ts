import { http } from './http'
import type {
  MetricsTimeseries,
  QueueTimeHistogram,
  WidgetCatalogEntry,
  WidgetDataParams,
} from '@/types'

export type MetricsRange = '1h' | '6h' | '24h' | '7d'

interface WidgetCatalogResponse {
  widgets?: WidgetCatalogEntry[]
}

export const metricsService = {
  getTimeseries(range: MetricsRange = '6h') {
    return http.get<MetricsTimeseries>(`/dashboard/metrics?range=${encodeURIComponent(range)}`)
  },
  // Delivery queue-time distribution; mailclass empty = global (all classes).
  getQueueTimeHistogram(range: MetricsRange = '6h', mailclass = '') {
    const query: Record<string, string> = { range }
    if (mailclass) query.mailclass = mailclass
    return http.get<QueueTimeHistogram>('/dashboard/queue-time-histogram', { query })
  },
  // Curated metric widget catalog for the dashboard builder.
  async getWidgetCatalog(): Promise<WidgetCatalogEntry[]> {
    const res = await http.get<WidgetCatalogResponse>('/dashboard/widget-catalog')
    return res.widgets ?? []
  },
  // One widget's data (catalog or guarded raw PromQL), in the shared timeseries
  // shape. Empty series / prometheusAvailable=false are normal, not errors.
  getWidgetData(params: WidgetDataParams) {
    const query: Record<string, string> = { source: params.source }
    if (params.catalogKey) query.catalogKey = params.catalogKey
    if (params.promql) query.promql = params.promql
    if (params.range) query.range = params.range
    if (params.groupBy) query.groupBy = params.groupBy
    return http.get<MetricsTimeseries>('/dashboard/widget-data', { query })
  },
}
