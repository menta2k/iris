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

function mockApiPlugin(): Plugin {
  return {
    name: 'iris-mock-api',
    configureServer(server) {
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
          },
    },
  }
})
