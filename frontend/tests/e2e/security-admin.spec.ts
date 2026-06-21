import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('Security admin story', () => {
  test('users, access and audit pages load', async ({ page }) => {
    let ok = await gotoApp(page, '/security/users')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()

    ok = await gotoApp(page, '/security/access')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'MFA & Permissions' })).toBeVisible()

    ok = await gotoApp(page, '/security/audit')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Audit Log' })).toBeVisible()
  })
})
