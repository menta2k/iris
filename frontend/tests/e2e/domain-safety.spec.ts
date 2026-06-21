import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('Domain safety story', () => {
  test('DKIM and suppressions pages load', async ({ page }) => {
    let ok = await gotoApp(page, '/domain-safety/dkim')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'DKIM Domains' })).toBeVisible()

    ok = await gotoApp(page, '/domain-safety/suppressions')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Suppressions' })).toBeVisible()
  })
})
