import { fileURLToPath, URL } from 'node:url'
import type { IncomingMessage, ServerResponse } from 'node:http'
import { defineConfig, loadEnv, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'
import { dispatch } from './src/mocks/router'

// When VITE_MOCK=true the dev server answers every /v1/* request from the
// in-memory mock DB (see src/mocks/) instead of proxying to the Go backend, so
// the UI can be developed/polished with no backend running.

function readMockBody(req: IncomingMessage): Promise<unknown> {
  return new Promise((resolve) => {
    const chunks: Buffer[] = []
    req.on('data', (chunk: Buffer) => chunks.push(chunk))
    req.on('end', () => {
      const raw = Buffer.concat(chunks).toString('utf-8')
      if (!raw) return resolve(undefined)
      try {
        resolve(JSON.parse(raw))
      } catch {
        resolve(raw)
      }
    })
    req.on('error', () => resolve(undefined))
  })
}

// mockSSEEvent builds a synthetic event for a given SSE stream (dev only).
let mockSeq = 0
function mockSSEEvent(stream: string): unknown {
  mockSeq += 1
  const now = new Date().toISOString()
  const domains = ['gmail.com', 'yahoo.com', 'abv.bg', 'mail.bg']
  const dom = domains[mockSeq % domains.length]
  if (stream === 'bounces') {
    return {
      id: `sse_b_${mockSeq}`,
      eventTime: now,
      recipient: `user${mockSeq}@${dom}`,
      mailclass: 'newsletter',
      smtpStatus: '550',
      bounceType: 'hard',
      diagnostic: 'Mailbox does not exist',
      processingState: 'new',
    }
  }
  if (stream === 'dashboard') {
    return { kind: mockSeq % 3 === 0 ? 'bounce' : 'mail' }
  }
  // mail-logs
  const sent = mockSeq % 4 !== 0
  return {
    id: `sse_m_${mockSeq}`,
    messageId: `msg-${mockSeq}`,
    eventTime: now,
    mailclass: 'newsletter',
    sender: 'news@example.com',
    fromHeader: 'Example <news@example.com>',
    recipient: `user${mockSeq}@${dom}`,
    recipientDomain: dom,
    vmtaId: '',
    egressSource: 'vmta-01',
    status: sent ? 'sent' : 'deferred',
    recordType: sent ? 'Delivery' : 'TransientFailure',
    smtpStatus: sent ? '250' : '451',
    diagnostic: '',
  }
}

function mockApiPlugin(): Plugin {
  return {
    name: 'iris-mock-api',
    configureServer(server) {
      server.middlewares.use((req: IncomingMessage, res: ServerResponse, next) => {
        // Mock SSE: emit synthetic real-time events so the Live toggles work
        // without a backend. The stream id is in the ?stream= query param.
        const raw = req.url ?? '/'
        if (!raw.startsWith('/sse')) return next()
        const stream = new URL(raw, 'http://localhost').searchParams.get('stream') ?? 'mail-logs'
        res.writeHead(200, {
          'Content-Type': 'text/event-stream',
          'Cache-Control': 'no-cache',
          Connection: 'keep-alive',
        })
        res.write(': connected\n\n')
        const timer = setInterval(() => {
          res.write(`data: ${JSON.stringify(mockSSEEvent(stream))}\n\n`)
        }, 3500)
        req.on('close', () => clearInterval(timer))
        return
      })
      server.middlewares.use(async (req: IncomingMessage, res: ServerResponse, next) => {
        const url = req.url ?? '/'
        if (!url.startsWith('/v1')) return next()
        const method = (req.method ?? 'GET').toUpperCase()
        const hasBody = method === 'POST' || method === 'PUT' || method === 'PATCH'
        const body = hasBody ? await readMockBody(req) : undefined
        const authHeader = req.headers.authorization
        const token = Array.isArray(authHeader) ? (authHeader[0] ?? null) : (authHeader ?? null)
        const result = dispatch(method, url, body, token)
        res.statusCode = result.status
        res.setHeader('Content-Type', 'application/json')
        res.end(result.body === undefined ? '' : JSON.stringify(result.body))
      })
      server.config.logger.info(
        '\n  \x1b[36m[iris-mock]\x1b[0m /v1/* is mocked — backend not required.\n' +
          '  \x1b[36m[iris-mock]\x1b[0m Any email + password logs in as admin. ' +
          'Set VITE_MOCK=false to use the real backend.\n',
        { timestamp: false },
      )
    },
  }
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd())
  const mockEnabled = env.VITE_MOCK === 'true'
  return {
    plugins: [
      vue(),
      // configFile compiles Vuetify from SASS so its CSS lands consistently.
      vuetify({ styles: { configFile: 'src/styles/settings.scss' } }),
      ...(mockEnabled ? [mockApiPlugin()] : []),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 5173,
      // When mocking, handle /v1 in-process; otherwise proxy to the Go backend.
      proxy: mockEnabled
        ? undefined
        : {
            '/v1': { target: 'http://localhost:8080', changeOrigin: true },
            '/openapi.yaml': { target: 'http://localhost:8080', changeOrigin: true },
            // SSE must not be buffered by the proxy.
            '/sse': { target: 'http://localhost:8080', changeOrigin: true },
          },
    },
  }
})
