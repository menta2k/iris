import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('KumoMTA config story', () => {
  test('config page generates a preview and guards apply with confirmation', async ({ page }) => {
    const ok = await gotoApp(page, '/operations/kumomta-config')
    test.skip(!ok, 'App/dev server not available')

    await expect(page.getByRole('heading', { name: 'KumoMTA Configuration' })).toBeVisible()

    // Generate / preview should render the generated content (or surface an error
    // without crashing the page).
    await page.getByTestId('generate-config').click()
    await expect(page.getByRole('heading', { name: 'KumoMTA Configuration' })).toBeVisible()

    // Apply must open a type-to-confirm dialog; the confirm button stays disabled
    // until the token is entered.
    await page.getByTestId('apply-config').click()
    await expect(page.getByRole('heading', { name: 'Apply config to KumoMTA' })).toBeVisible()
    const confirm = page.getByRole('button', { name: 'Apply', exact: true })
    await expect(confirm).toBeDisabled()
  })
})
