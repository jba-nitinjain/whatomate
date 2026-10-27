import { test, expect } from '@playwright/test'
import { loginAsAdmin, loginAsAgent } from '../../helpers'

/**
 * Chatbot Conversation History (chatbot sessions) E2E.
 *
 * Sessions are created only by inbound WhatsApp traffic, so these specs don't
 * seed data: they cover the menu entry, the list page in either its empty or
 * populated state, the filters hitting the API, and the permission gate.
 */

test.describe('Chatbot Conversation History', () => {
  test('menu shows Conversation History under Chatbot', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/chatbot')
    await page.waitForLoadState('networkidle')

    const navLink = page.locator('nav a[href="/chatbot/sessions"]').first()
    await expect(navLink).toBeVisible()
    await expect(navLink).toContainText('Conversation History')
    await navLink.click()
    await expect(page).toHaveURL(/\/chatbot\/sessions$/)
  })

  test('list page loads with table or empty state', async ({ page }) => {
    await loginAsAdmin(page)
    const listResponse = page.waitForResponse(
      (r) => r.url().includes('/api/chatbot/sessions') && r.request().method() === 'GET'
    )
    await page.goto('/chatbot/sessions')
    expect((await listResponse).status()).toBe(200)
    await page.waitForLoadState('networkidle')

    await expect(page.getByRole('heading', { level: 1 })).toContainText('Conversation History')
    await expect(page.getByTestId('sessions-search').locator('input')).toBeVisible()

    const rows = page.getByTestId('session-row-link')
    const empty = page.getByText('No conversations found')
    await expect(rows.first().or(empty)).toBeVisible()
  })

  test('status filter sends the status to the API', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/chatbot/sessions')
    await page.waitForLoadState('networkidle')

    const filtered = page.waitForResponse(
      (r) => r.url().includes('/api/chatbot/sessions') && r.url().includes('status=completed')
    )
    await page.getByTestId('sessions-status-filter').click()
    await page.getByRole('option', { name: /^Completed$/ }).click()
    expect((await filtered).status()).toBe(200)
  })

  test('opening a conversation shows its details', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/chatbot/sessions')
    await page.waitForLoadState('networkidle')

    const firstRow = page.getByTestId('session-row-link').first()
    test.skip(!(await firstRow.isVisible()), 'No chatbot sessions in this environment')

    await firstRow.click()
    await expect(page).toHaveURL(/\/chatbot\/sessions\/[a-f0-9-]+$/)
    await expect(page.getByTestId('session-details')).toBeVisible()
    await expect(page.getByText('Collected Answers')).toBeVisible()
    await expect(page.getByText('Message Log')).toBeVisible()
    await expect(page.getByTestId('session-open-chat')).toHaveAttribute('href', /\/chat\/[a-f0-9-]+/)
  })

  test('agent without chatbot flow access cannot open the page', async ({ page }) => {
    await loginAsAgent(page)
    await page.goto('/chatbot/sessions')
    await page.waitForLoadState('networkidle')
    await expect(page).not.toHaveURL(/\/chatbot\/sessions/)
  })
})
