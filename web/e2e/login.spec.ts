import { test, expect } from '@playwright/test';

test.describe('Login Page', () => {
  test('renders login form with tenant, token, and submit button', async ({ page }) => {
    await page.goto('/login');

    await expect(page.locator('#tenant')).toBeVisible();
    await expect(page.locator('#token')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('shows error for empty credentials', async ({ page }) => {
    await page.goto('/login');

    await page.click('button[type="submit"]');

    // Wait for error message to appear
    const errorLocator = page.locator('.text-red-400');
    await expect(errorLocator).toBeVisible({ timeout: 5000 });
  });

  test('login form accepts input and submit', async ({ page }) => {
    await page.goto('/login');

    await page.fill('#tenant', 'nest');
    await page.fill('#token', 'test-token-12345');

    // Verify inputs are filled
    const tenantValue = await page.inputValue('#tenant');
    const tokenValue = await page.inputValue('#token');
    expect(tenantValue).toBe('nest');
    expect(tokenValue).toBe('test-token-12345');

    // Verify submit button is clickable
    const submitButton = page.locator('button[type="submit"]');
    await expect(submitButton).toBeEnabled();
  });
});
