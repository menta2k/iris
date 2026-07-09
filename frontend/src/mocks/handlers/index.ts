// Aggregates every handler module's routes into the single route table the
// router dispatches against. Order matters only for overlapping patterns; the
// domains here have disjoint path prefixes so ordering is not significant.

import type { Route } from '../router'
import { authRoutes } from './auth'
import { bounceRulesRoutes } from './bounce-rules'
import { dashboardRoutes } from './dashboard'
import { eventProcessorsRoutes } from './event-processors'
import { domainSafetyRoutes } from './domain-safety'
import { inboundHandlers } from './inbound'
import { operationsRoutes } from './operations'
import { outboundRoutes } from './outbound'
import { securityRoutes } from './security'
import { settingsRoutes } from './settings'
import { systemMonitorRoutes } from './system-monitor'
import { toolsRoutes } from './tools'

export const routes: Route[] = [
  ...authRoutes,
  ...dashboardRoutes,
  ...outboundRoutes,
  ...bounceRulesRoutes,
  ...eventProcessorsRoutes,
  ...operationsRoutes,
  ...domainSafetyRoutes,
  ...inboundHandlers,
  ...securityRoutes,
  ...settingsRoutes,
  ...systemMonitorRoutes,
  ...toolsRoutes,
]
