import { test, expect } from '@playwright/test'
import { loginAsAdmin, loginAsAgent } from '../../helpers'

/**
 * Product catalogs E2E.
 *
 * Creating catalogs or products round-trips to Meta, which the e2e stack does
 * not reach, so these specs cover what works offline: the menu entry, the list
 * page shell, and the create / sync dialogs including client-side validation.
 */

test.describe('Product Catalogs', () => {
  test('admin sees the Catalogs menu item and the list page loads', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/')
    await page.waitForSelector('aside nav')
    await expect(page.locator('aside nav').getByText('Catalogs', { exact: true })).toBeVisible()

    await page.locator('aside nav').getByText('Catalogs', { exact: true }).click()
    await page.waitForURL(/\/catalogs$/)
    await page.waitForLoadState('networkidle')

    await expect(page.getByRole('heading', { level: 1 })).toContainText('Product Catalogs')
    await expect(page.getByText('Your Catalogs')).toBeVisible()
    await expect(page.getByTestId('catalog-account-filter')).toBeVisible()
  })

  test('create catalog dialog validates the name', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/catalogs')
    await page.waitForLoadState('networkidle')

    await page.getByRole('button', { name: 'Create Catalog' }).first().click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('Create Catalog')

    await dialog.getByRole('button', { name: 'Create', exact: true }).click()
    await expect(page.locator('[data-sonner-toast]').filter({ hasText: 'Please enter a name' })).toBeVisible()
  })

  test('sync from Meta dialog asks for a WhatsApp account', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/catalogs')
    await page.waitForLoadState('networkidle')

    await page.getByRole('button', { name: 'Sync from Meta' }).first().click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toContainText('Sync Catalogs from Meta')
    await expect(dialog.getByTestId('catalog-sync-account-select')).toBeVisible()
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).toBeHidden()
  })

  test('agent without catalogs permission does not see the menu item', async ({ page }) => {
    await loginAsAgent(page)
    await page.waitForSelector('aside nav')
    await expect(page.locator('aside nav').getByText('Catalogs', { exact: true })).toHaveCount(0)
  })
})
