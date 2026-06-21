import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('Dashboard story', () => {
  test('renders dashboard widgets without crashing', async ({ page }) => {
    const ok = await gotoApp(page, '/')
    test.skip(!ok, 'App/dev server not available')

    // The page renders without crashing even when the backend is unavailable:
    // either the service-status widget (backend up) or a friendly error/empty
    // state (backend down) is shown — never a blank crash.
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
    const widget = page.getByText('KumoMTA Service')
    const fallback = page.getByText(/Cannot reach the backend|not available yet|Request failed|Failed to load/)
    await expect(widget.or(fallback).first()).toBeVisible()
  })
})
