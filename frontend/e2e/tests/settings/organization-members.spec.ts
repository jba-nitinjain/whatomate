import { test, expect } from '@playwright/test'
import { loginAsAdmin, loginAsAgent, ApiHelper } from '../../helpers'
import { createTestScope, createUserWithPermissions, type TestUserHandle } from '../../framework'

const scope = createTestScope('org-members')

test.describe('Organisation Members - List', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings/organization-members')
    await page.waitForLoadState('networkidle')
  })

  test('shows the page, current organisation and the signed-in admin', async ({ page }) => {
    await expect(page.getByRole('heading', { level: 1 })).toContainText(/Organisation Members/i)
    await expect(page.getByTestId('current-organization')).toBeVisible()
    await expect(page.locator('tbody')).toBeVisible()
    await expect(page.locator('tbody tr').filter({ hasText: 'You' }).first()).toBeVisible()
  })

  test('is reachable from the Settings menu', async ({ page }) => {
    await page.goto('/settings')
    await page.waitForLoadState('networkidle')
    const link = page.locator('a[href$="/settings/organization-members"]').first()
    await expect(link).toBeVisible()
    await link.click()
    await expect(page).toHaveURL(/\/settings\/organization-members$/)
  })

  test('shows the backend error when the email has no account', async ({ page }) => {
    await page.getByRole('button', { name: /Add Member/i }).first().click()
    const dialog = page.locator('[role="dialog"]')
    await expect(dialog).toBeVisible()
    await dialog.locator('input[type="email"]').fill(`${scope.name('nobody')}@example.invalid`)
    await dialog.getByRole('button', { name: /^Add Member$/i }).click()
    await expect(page.locator('[data-sonner-toast]').filter({ hasText: /No user found/i })).toBeVisible()
  })
})

test.describe('Organisation Members - Remove and re-add', () => {
  test.describe.configure({ mode: 'serial' })

  let api: ApiHelper
  let member: TestUserHandle

  test.beforeAll(async ({ request }) => {
    api = new ApiHelper(request)
    await api.loginAsAdmin()
    member = await createUserWithPermissions(api, scope, {
      userSlug: 'member',
      permissions: [{ resource: 'chat', action: 'read' }],
    })
  })

  test.afterAll(async () => {
    if (member) {
      await api.deleteUser(member.user.id).catch(() => {})
      await api.deleteRole(member.role.id).catch(() => {})
    }
  })

  test('removes a member after confirmation and adds them back by email', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings/organization-members')
    await page.waitForLoadState('networkidle')

    await page.getByPlaceholder(/Search by name or email/i).fill(member.email)
    const row = page.locator('tbody tr').filter({ hasText: member.email })
    await expect(row).toBeVisible()

    await row.getByRole('button', { name: /Remove from organisation/i }).click()
    const confirm = page.locator('[role="alertdialog"]')
    await expect(confirm).toBeVisible()
    await confirm.getByRole('button', { name: /^Remove$/ }).click()
    await expect(page.locator('[data-sonner-toast]').filter({ hasText: /Member removed/i })).toBeVisible()
    await expect(row).toHaveCount(0)

    await page.getByRole('button', { name: /Add Member/i }).first().click()
    const dialog = page.locator('[role="dialog"]')
    await dialog.locator('input[type="email"]').fill(member.email)
    await dialog.getByRole('button', { name: /^Add Member$/i }).click()
    await expect(page.locator('[data-sonner-toast]').filter({ hasText: /Member added/i })).toBeVisible()
    await expect(page.locator('tbody tr').filter({ hasText: member.email })).toBeVisible()
  })
})

test.describe('Organisation Members - Permissions', () => {
  test('agents without the organisations permission do not see the menu item', async ({ page }) => {
    await loginAsAgent(page)
    await page.goto('/settings/organization-members')
    await page.waitForLoadState('networkidle')
    await expect(page).not.toHaveURL(/\/settings\/organization-members$/)
    await expect(page.locator('a[href$="/settings/organization-members"]')).toHaveCount(0)
  })
})
