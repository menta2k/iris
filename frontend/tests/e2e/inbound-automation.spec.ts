import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('Inbound automation story', () => {
  test('inbound routes and rspamd pages load', async ({ page }) => {
    let ok = await gotoApp(page, '/inbound/routes')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Inbound Routes' })).toBeVisible()

    ok = await gotoApp(page, '/inbound/rspamd')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Rspamd Results' })).toBeVisible()
  })
})
