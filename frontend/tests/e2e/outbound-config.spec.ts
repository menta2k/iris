import { test, expect } from '@playwright/test'
import { gotoApp } from './helpers'

test.describe('Outbound config story', () => {
  test('Listeners page loads and exposes its create dialog', async ({ page }) => {
    const ok = await gotoApp(page, '/outbound/listeners')
    test.skip(!ok, 'App/dev server not available')

    await expect(page.getByRole('heading', { name: 'Listeners' })).toBeVisible()
    await page.getByTestId('create-listener').click()
    await expect(page.getByRole('heading', { name: 'Create Listener' })).toBeVisible()
    await expect(page.locator('#listener-port')).toBeVisible()
  })

  test('VMTAs page lets an operator open the create dialog with a listener dropdown', async ({
    page,
  }) => {
    const ok = await gotoApp(page, '/outbound/vmtas')
    test.skip(!ok, 'App/dev server not available')

    await expect(page.getByRole('heading', { name: 'VMTAs' })).toBeVisible()
    await page.getByTestId('create-vmta').click()
    await expect(page.getByRole('heading', { name: 'Create VMTA' })).toBeVisible()
    // The create form replaces ip/ehlo inputs with a Listener dropdown.
    await expect(page.getByTestId('vmta-listener')).toBeVisible()
  })

  test('VMTAs page lets an operator open an edit dialog when rows exist', async ({ page }) => {
    const ok = await gotoApp(page, '/outbound/vmtas')
    test.skip(!ok, 'App/dev server not available')

    await expect(page.getByRole('heading', { name: 'VMTAs' })).toBeVisible()

    // Edit buttons only render when the list has rows (backend may be empty).
    const editButton = page.locator('[data-testid^="edit-vmta-"]').first()
    const hasRows = await editButton.count()
    test.skip(hasRows === 0, 'No VMTAs available to edit')

    await editButton.click()
    await expect(page.getByRole('heading', { name: 'Edit VMTA' })).toBeVisible()
    // Edit form exposes the editable status select.
    await expect(page.locator('#vmta-status')).toBeVisible()
  })

  test('VMTA groups and routing rules pages load', async ({ page }) => {
    let ok = await gotoApp(page, '/outbound/vmta-groups')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'VMTA Groups' })).toBeVisible()

    ok = await gotoApp(page, '/outbound/routing-rules')
    test.skip(!ok, 'App/dev server not available')
    await expect(page.getByRole('heading', { name: 'Routing Rules' })).toBeVisible()
  })
})
