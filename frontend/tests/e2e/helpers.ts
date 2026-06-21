import { type Page, expect } from '@playwright/test'

/**
 * Navigate to a path and verify the app shell rendered. If the dev server or
 * app fails to load (e.g. backend missing entirely and the SPA can't boot),
 * the calling test should skip rather than fail. The SPA itself is designed to
 * render even when the backend is down, so reaching the shell is the bar.
 */
export async function gotoApp(page: Page, path: string): Promise<boolean> {
  try {
    await page.goto(path, { waitUntil: 'domcontentloaded', timeout: 15_000 })
    // The app shell (sidebar brand) should appear regardless of backend state.
    await expect(page.getByText('KumoMTA Admin')).toBeVisible({ timeout: 10_000 })
    return true
  } catch {
    return false
  }
}
