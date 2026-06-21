import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('Mail operations story', () => {
  test('mail logs page exposes filters', async ({ page }) => {
    const ok = await gotoApp(page, '/operations/mail-logs')
    test.skip(!ok, 'App/dev server not available')

    await expect(page.getByRole('heading', { name: 'Mail Logs' })).toBeVisible()
    await page.getByPlaceholder('marketing').fill('transactional')
    await page.getByTestId('apply-filters').click()
    // Page should not crash; either rows or an empty state is shown.
    await expect(page.getByRole('heading', { name: 'Mail Logs' })).toBeVisible()
  })

  test('service control requires confirmation for restart', async ({ page }) => {
    const ok = await gotoApp(page, '/operations/service-control')
    test.skip(!ok, 'App/dev server not available')

    await expect(page.getByRole('heading', { name: 'Service Control' })).toBeVisible()
    await page.getByTestId('svc-restart').click()
    // A typed-confirmation dialog must guard the destructive restart.
    await expect(page.getByRole('heading', { name: 'Restart KumoMTA' })).toBeVisible()
    const confirm = page.getByRole('button', { name: 'Restart', exact: true })
    await expect(confirm).toBeDisabled()
  })

  test('queues page loads', async ({ page }) => {
    const ok = await gotoApp(page, '/operations/queues')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Queues' })).toBeVisible()
  })
})
