import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('Inbound automation story', () => {
  test('webhook, delivery events and rspamd pages load', async ({ page }) => {
    let ok = await gotoApp(page, '/inbound/webhooks')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Webhook Rules' })).toBeVisible()

    ok = await gotoApp(page, '/inbound/delivery-events')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Delivery Events' })).toBeVisible()

    ok = await gotoApp(page, '/inbound/rspamd')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Rspamd Results' })).toBeVisible()
  })
})
