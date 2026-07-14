// KumoMTA cluster node fixtures.

import type { MTANode } from '../../types'
import { daysAgo } from './util'

export const mtaNodes: MTANode[] = [
  {
    id: 'node-0001',
    name: 'mta-local',
    agentUrl: '',
    proxyHost: '',
    proxyPort: 0,
    status: 'active',
    certFingerprint: '',
    kumoState: 'running',
    version: '',
    appliedChecksum: 'a3f9c2e1',
    lastSeenAt: daysAgo(0),
    notes: 'Co-located node managed through the local file/reload transport.',
  },
  {
    id: 'node-0002',
    name: 'mta-eu-2',
    agentUrl: 'https://10.20.0.12:8447',
    proxyHost: '10.20.0.12',
    proxyPort: 1080,
    status: 'active',
    certFingerprint: '9c1c6f3f7a2e4b8d9c1c6f3f7a2e4b8d9c1c6f3f7a2e4b8d9c1c6f3f7a2e4b8d',
    kumoState: 'running',
    version: 'iris-agent/1',
    appliedChecksum: 'a3f9c2e1',
    lastSeenAt: daysAgo(0),
    notes: '',
  },
  {
    id: 'node-0003',
    name: 'mta-eu-3',
    agentUrl: 'https://10.20.0.13:8447',
    proxyHost: '10.20.0.13',
    proxyPort: 1080,
    status: 'draining',
    certFingerprint: '5e8d4a2b6c9f1e3d5e8d4a2b6c9f1e3d5e8d4a2b6c9f1e3d5e8d4a2b6c9f1e3d',
    kumoState: 'unreachable',
    version: 'iris-agent/1',
    appliedChecksum: '77b0d4f2', // differs from expected -> drift badge in the UI
    lastSeenAt: daysAgo(2),
    notes: 'Being drained for kernel maintenance.',
  },
]
