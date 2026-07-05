// Outbound configuration fixtures: listeners, VMTAs, VMTA groups, routing rules,
// IP warmup schedules, delivery blueprints, and TSA automation rules.
// IDs are stable so entities can reference each other (VMTA→listener, etc.).

import type {
  AutomationRule,
  DeliveryBlueprint,
  Listener,
  RoutingRule,
  VMTA,
  VMTAGroup,
  WarmupSchedule,
} from '../../types'

export const listeners: Listener[] = [
  {
    id: 'lst_main',
    name: 'mta-inbound-1',
    ipAddress: '10.0.0.11',
    port: 25,
    hostname: 'mta1.example.net',
    tlsEnabled: true,
    tlsCertPath: '/etc/iris/tls/mta1.fullchain.pem',
    tlsKeyPath: '/etc/iris/tls/mta1.key',
    requireAuth: false,
    maxMessageSize: '26214400',
    relayHosts: ['10.0.0.0/8'],
    status: 'active',
    role: 'inbound',
  },
  {
    id: 'lst_submit',
    name: 'msa-submission',
    ipAddress: '10.0.0.11',
    port: 587,
    hostname: 'submit.example.net',
    tlsEnabled: true,
    tlsCertPath: '/etc/iris/tls/submit.fullchain.pem',
    tlsKeyPath: '/etc/iris/tls/submit.key',
    requireAuth: true,
    maxMessageSize: '26214400',
    relayHosts: ['10.0.0.0/8'],
    status: 'active',
    role: 'submission',
  },
  {
    id: 'lst_mta2',
    name: 'mta-inbound-2',
    ipAddress: '10.0.0.12',
    port: 25,
    hostname: 'mta2.example.net',
    tlsEnabled: true,
    tlsCertPath: '/etc/iris/tls/mta2.fullchain.pem',
    tlsKeyPath: '/etc/iris/tls/mta2.key',
    requireAuth: false,
    maxMessageSize: '26214400',
    relayHosts: ['10.0.0.0/8'],
    status: 'active',
    role: 'inbound',
  },
  {
    id: 'lst_mta3',
    name: 'mta-inbound-3',
    ipAddress: '10.0.0.13',
    port: 25,
    hostname: 'mta3.example.net',
    tlsEnabled: false,
    tlsCertPath: '',
    tlsKeyPath: '',
    requireAuth: false,
    maxMessageSize: '26214400',
    relayHosts: ['10.0.0.0/8'],
    status: 'disabled',
    role: 'inbound',
  },
]

export const vmtas: VMTA[] = [
  { id: 'vmta1', name: 'promo-1', status: 'ACTIVE', notes: 'Promo pool primary', listenerId: 'lst_main', listenerName: 'mta-inbound-1', ipAddress: '203.0.113.11', ehloName: 'mta1.example.net', maxConnections: 200 },
  { id: 'vmta2', name: 'promo-2', status: 'ACTIVE', notes: 'Promo pool', listenerId: 'lst_mta2', listenerName: 'mta-inbound-2', ipAddress: '203.0.113.12', ehloName: 'mta2.example.net', maxConnections: 200 },
  { id: 'vmta3', name: 'promo-3', status: 'ACTIVE', notes: '', listenerId: 'lst_main', listenerName: 'mta-inbound-1', ipAddress: '203.0.113.13', ehloName: 'mta3.example.net', maxConnections: 150 },
  { id: 'vmta4', name: 'transac-1', status: 'ACTIVE', notes: 'Transactional', listenerId: 'lst_mta2', listenerName: 'mta-inbound-2', ipAddress: '203.0.113.21', ehloName: 'mta1.example.net', maxConnections: 300 },
  { id: 'vmta5', name: 'transac-2', status: 'DRAINING', notes: 'Draining for maintenance', listenerId: 'lst_main', listenerName: 'mta-inbound-1', ipAddress: '203.0.113.22', ehloName: 'mta2.example.net', maxConnections: 300 },
  { id: 'vmta6', name: 'warmup-1', status: 'ACTIVE', notes: 'IP warming up', listenerId: 'lst_mta2', listenerName: 'mta-inbound-2', ipAddress: '203.0.113.31', ehloName: 'mta3.example.net', maxConnections: 50 },
  { id: 'vmta7', name: 'newsletter-1', status: 'DISABLED', notes: 'Paused by ops', listenerId: 'lst_mta3', listenerName: 'mta-inbound-3', ipAddress: '203.0.113.41', ehloName: 'mta3.example.net', maxConnections: 120 },
]

export const vmtaGroups: VMTAGroup[] = [
  {
    id: 'grp_promo',
    name: 'promo-pool',
    status: 'ACTIVE',
    members: [
      { vmtaId: 'vmta1', weight: 40 },
      { vmtaId: 'vmta2', weight: 40 },
      { vmtaId: 'vmta3', weight: 20 },
    ],
  },
  {
    id: 'grp_transac',
    name: 'transactional-pool',
    status: 'ACTIVE',
    members: [
      { vmtaId: 'vmta4', weight: 60 },
      { vmtaId: 'vmta5', weight: 40 },
    ],
  },
]

export const routingRules: RoutingRule[] = [
  { id: 'rule_promo', name: 'Promo campaigns', matchType: 'mailclass', matchHeader: 'X-Campaign-Type', matchValue: 'promo', priority: 100, targetType: 'vmta_group', targetId: 'grp_promo', status: 'active' },
  { id: 'rule_transac', name: 'Transactional mail', matchType: 'mailclass', matchHeader: 'X-Campaign-Type', matchValue: 'transactional', priority: 90, targetType: 'vmta_group', targetId: 'grp_transac', status: 'active' },
  { id: 'rule_newsletter', name: 'Newsletters', matchType: 'mailclass', matchHeader: 'X-Campaign-Type', matchValue: 'newsletter', priority: 80, targetType: 'vmta', targetId: 'vmta7', status: 'active' },
  { id: 'rule_bouncedomain', name: 'Bounce domain rule', matchType: 'recipient_domain', matchValue: 'yahoo.com', priority: 50, targetType: 'vmta_group', targetId: 'grp_promo', status: 'active' },
  { id: 'rule_senderip', name: 'Internal relay', matchType: 'sender_ip', matchValue: '10.0.0.0/8', priority: 30, targetType: 'vmta', targetId: 'vmta4', assignMailclass: 'transactional', status: 'active' },
  { id: 'rule_disabled', name: 'Legacy rule', matchType: 'recipient_domain', matchValue: 'old.example.com', priority: 10, targetType: 'vmta', targetId: 'vmta7', status: 'disabled' },
]

export const warmupSchedules: WarmupSchedule[] = [
  {
    id: 'wrm_vmta6',
    vmtaId: 'vmta6',
    vmtaName: 'warmup-1',
    startDate: '2026-06-20',
    curve: 'standard-30',
    status: 'active',
    stages: [
      { dayFrom: 1, dayTo: 3, caps: { gmail: 500, yahoo: 200, outlook: 100 } },
      { dayFrom: 4, dayTo: 7, caps: { gmail: 2000, yahoo: 800, outlook: 400 } },
      { dayFrom: 8, dayTo: 14, caps: { gmail: 8000, yahoo: 3000, outlook: 1500 } },
      { dayFrom: 15, dayTo: 30, caps: { gmail: 30000, yahoo: 12000, outlook: 6000 } },
    ],
  },
  {
    id: 'wrm_vmta3',
    vmtaId: 'vmta3',
    vmtaName: 'promo-3',
    startDate: '2026-06-01',
    curve: 'standard-30',
    status: 'completed',
    stages: [
      { dayFrom: 1, dayTo: 30, caps: { gmail: 30000, yahoo: 12000, outlook: 6000 } },
    ],
  },
  {
    id: 'wrm_vmta7',
    vmtaId: 'vmta7',
    vmtaName: 'newsletter-1',
    startDate: '2026-06-25',
    curve: 'custom',
    status: 'paused',
    pausedReason: 'High bounce rate on yahoo.com',
    heldDay: 5,
    stages: [
      { dayFrom: 1, dayTo: 5, caps: { gmail: 1000, yahoo: 400 } },
      { dayFrom: 6, dayTo: 14, caps: { gmail: 5000, yahoo: 2000 } },
    ],
  },
]

export const blueprints: DeliveryBlueprint[] = [
  { id: 'bp_gmail', provider: 'Gmail', mxPattern: 'google.*|gmail.*', connRate: '10/s', deliveriesPerConn: 100, connLimit: 40, dailyCap: 50000, status: 'active' },
  { id: 'bp_yahoo', provider: 'Yahoo', mxPattern: 'yahoo.*|yahoodns.*', connRate: '5/s', deliveriesPerConn: 50, connLimit: 20, dailyCap: 20000, status: 'active' },
  { id: 'bp_outlook', provider: 'Outlook', mxPattern: 'outlook.*|hotmail.*', connRate: '8/s', deliveriesPerConn: 60, connLimit: 30, dailyCap: 25000, status: 'active' },
  { id: 'bp_apple', provider: 'Apple', mxPattern: 'apple.*|icloud.*', connRate: '6/s', deliveriesPerConn: 40, connLimit: 15, dailyCap: 15000, status: 'active' },
  { id: 'bp_default', provider: 'Default', mxPattern: '.*', connRate: '4/s', deliveriesPerConn: 30, connLimit: 10, dailyCap: 10000, status: 'disabled' },
]

export const automationRules: AutomationRule[] = [
  { id: 'auto_defer', domain: 'yahoo.com', regex: '421 4\\.7\\.', action: 'suspend', trigger: 'defer_rate>20%', duration: '1h', status: 'active' },
  { id: 'auto_block', domain: 'gmail.com', regex: '550 5\\.1\\.1', action: 'set_config', configName: 'suppression', configValue: 'recipient', trigger: 'hard_bounce', duration: 'permanent', status: 'active' },
  { id: 'auto_greylist', domain: '*', regex: '451 4\\.7\\.1', action: 'suspend', trigger: 'defer_rate>50%', duration: '30m', status: 'disabled' },
  { id: 'auto_warmup_hold', domain: 'outlook.com', regex: '421 4\\.7\\.', action: 'suspend_tenant', trigger: 'defer_rate>10%', duration: '2h', status: 'active' },
]
