// Tools + KumoMTA config handlers: ACME account/certs/dns-providers, rendered
// config generate/apply/status, the Diagnose routing+DNS inspector, and RBL check.

import type {
  AcmeAccount,
  AcmeCertificate,
  AcmeDnsProvider,
  SaveAcmeAccountRequest,
} from '../../types'
import { all, createRow, genId, removeRow } from '../db'
import { acmeAccount, acmeDnsProvider, acmeDnsProviders } from '../fixtures/tools'
import { daysAgo, hoursAgo } from '../fixtures/util'
import { noContent, notFound, ok, type Route } from '../router'

let account: AcmeAccount = { ...acmeAccount }
let dnsProvider: AcmeDnsProvider | null = { ...acmeDnsProvider }

function domainFromEmail(email: string): string {
  const at = email.lastIndexOf('@')
  return at === -1 ? email : email.slice(at + 1)
}

function readinessItems(domain: string): Array<{ name: string; status: string; detail: string; records?: string[] }> {
  const ok2 = domain.endsWith('example.net') || domain.endsWith('example.com')
  return [
    { name: 'MX', status: ok2 ? 'pass' : 'fail', detail: ok2 ? '10 mta1.example.net.' : 'No MX', records: ok2 ? ['10 mta1.example.net.'] : [] },
    { name: 'SPF', status: ok2 ? 'pass' : 'warn', detail: ok2 ? 'v=spf1 include:_spf.example.net -all' : 'Missing SPF' },
    { name: 'DKIM', status: ok2 ? 'pass' : 'warn', detail: ok2 ? 'selector iris published' : 'DKIM missing' },
    { name: 'DMARC', status: ok2 ? 'pass' : 'fail', detail: ok2 ? 'p=quarantine' : 'DMARC missing' },
  ]
}

export const toolsRoutes: Route[] = [
  // ---- ACME account ----
  { method: 'GET', pattern: '/acme/account', handler: () => ok(account) },
  {
    method: 'PUT',
    pattern: '/acme/account',
    handler: (ctx) => {
      const body = ctx.body as SaveAcmeAccountRequest
      account = { ...account, email: body.email, serverUrl: body.server_url, configured: true, registered: true, updatedAt: hoursAgo(0) }
      return ok(account)
    },
  },

  // ---- ACME certificates ----
  { method: 'GET', pattern: '/acme/certificates', handler: () => ok({ items: all('acmeCertificates'), page: {} }) },
  {
    method: 'POST',
    pattern: '/acme/certificates',
    handler: (ctx) => {
      const body = ctx.body as { domain: string; alt_names: string[] }
      const cert: AcmeCertificate = {
        id: genId('crt'),
        domain: body.domain,
        altNames: body.alt_names ?? [],
        challengeType: 'dns-01',
        certPath: '',
        keyPath: '',
        expiresAt: daysAgo(-90),
        lastRenewedAt: '',
        status: 'pending',
        lastError: '',
      }
      return ok(createRow('acmeCertificates', cert))
    },
  },
  {
    method: 'DELETE',
    pattern: '/acme/certificates/:id',
    handler: (ctx) => (removeRow('acmeCertificates', ctx.params.id) ? noContent() : notFound('Certificate not found')),
  },

  // ---- ACME DNS providers ----
  { method: 'GET', pattern: '/acme/dns-providers', handler: () => ok({ items: acmeDnsProviders }) },
  { method: 'GET', pattern: '/acme/dns-provider', handler: () => ok(dnsProvider ?? { provider: '', config: {}, updatedAt: '' }) },
  {
    method: 'PUT',
    pattern: '/acme/dns-provider',
    handler: (ctx) => {
      const body = ctx.body as { provider: string; config: Record<string, string> }
      const redacted: Record<string, string> = {}
      for (const key of Object.keys(body.config)) redacted[key] = '[stored]'
      dnsProvider = { provider: body.provider, config: redacted, updatedAt: hoursAgo(0) }
      return ok(dnsProvider)
    },
  },
  {
    method: 'DELETE',
    pattern: '/acme/dns-provider',
    handler: () => {
      dnsProvider = null
      return ok({ provider: '', config: {}, updatedAt: '' })
    },
  },

  // ---- KumoMTA config ----
  {
    method: 'GET',
    pattern: '/kumomta/config:generate',
    handler: () => {
      const vmtaCount = all('vmtas').length
      const poolCount = all('vmtaGroups').length
      const routeCount = all('routingRules').length
      const dkimCount = all('dkimDomains').length
      const suppressionCount = all('suppressions').length
      const content = [
        '-- iris-rendered init.lua',
        'local kumo = require \'kumo\'',
        'kumo.on("get_egress_source", function(name) return kumo.make_egress_source(EGRESS_SOURCES[name]) end)',
        `-- ${vmtaCount} vmta(s), ${poolCount} pool(s), ${routeCount} route(s)`,
      ].join('\n')
      return ok({
        content,
        vmtaCount,
        poolCount,
        routeCount,
        dkimCount,
        suppressionCount,
        checksum: 'sha256:' + genId(''),
        valid: true,
        lintIssues: [],
      })
    },
  },
  {
    method: 'POST',
    pattern: '/kumomta/config:apply',
    handler: () => ok({ requestId: genId('req'), status: 'applied', checksum: 'sha256:' + genId(''), appliedPath: '/policy/init.lua', resultSummary: 'Policy rendered and kumod reloaded' }),
  },
  {
    method: 'GET',
    pattern: '/kumomta/config:status',
    handler: () => ok({ drift: false, neverApplied: false, currentChecksum: 'sha256:abc', appliedChecksum: 'sha256:abc', appliedAt: hoursAgo(2), restartRequired: false }),
  },

  // ---- Diagnose ----
  {
    method: 'POST',
    pattern: '/tools/diagnose',
    handler: (ctx) => {
      const body = ctx.body as { from_email: string; recipient?: string; mailclass?: string }
      const domain = domainFromEmail(body.recipient || body.from_email)
      return ok({
        fromEmail: body.from_email,
        domain,
        items: readinessItems(domain),
        routing: {
          matchedRule: 'rule_promo',
          egressPool: 'grp_promo',
          vmtas: ['promo-1', 'promo-2', 'promo-3'],
          egressIps: ['203.0.113.11', '203.0.113.12', '203.0.113.13'],
          listeners: ['mta-inbound-1'],
          note: `Routed via ${body.mailclass || 'default'} mail class`,
        },
      })
    },
  },

  // ---- RBL check ----
  {
    method: 'POST',
    pattern: '/tools/rbl-check',
    handler: () => {
      const zones = ['zen.spamhaus.org', 'bl.spamcop.net', 'dnsbl-1.uceprotect.net', 'b.barracudacentral.org']
      const ips = ['203.0.113.11', '203.0.113.21']
      return ok({
        results: ips.map((ip, i) => ({
          ip,
          source: i === 0 ? 'promo-1' : 'transac-1',
          listed: false,
          listings: zones.map((zone) => ({ zone, listed: false })),
        })),
        zones,
        checkedAt: hoursAgo(0),
        skipped: [],
      })
    },
  },
]
