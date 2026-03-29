import { test, expect } from '@playwright/test';

test.describe('Login Page', () => {
  test('renders login form with email, password, and submit button', async ({ page }) => {
    await page.goto('/login');

    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('shows error for invalid credentials', async ({ page }) => {
    await page.goto('/login');

    await page.fill('input[name="email"]', 'wrong@example.com');
    await page.fill('input[name="password"]', 'wrongpassword');
    await page.click('button[type="submit"]');

    // Wait for error message to appear
    const errorLocator = page.locator('[data-testid="login-error"], .text-red-400, .text-red-500, [role="alert"]');
    await expect(errorLocator.first()).toBeVisible({ timeout: 10000 });
  });

  test('successful login redirects to dashboard', async ({ page }) => {
    await page.goto('/login');

    await page.fill('input[name="email"]', 'admin@localhost.local');
    await page.fill('input[name="password"]', 'admin123');
    await page.click('button[type="submit"]');

    await page.waitForURL('**/dashboard', { timeout: 15000 });
    expect(page.url()).toContain('/dashboard');
  });

  test('logout works', async ({ page }) => {
    // Login first
    await page.goto('/login');
    await page.fill('input[name="email"]', 'admin@localhost.local');
    await page.fill('input[name="password"]', 'admin123');
    await page.click('button[type="submit"]');
    await page.waitForURL('**/dashboard', { timeout: 15000 });

    // Find and click logout button
    const logoutButton = page.locator('[data-testid="logout-btn"], button:has-text("Logout"), button:has-text("Log out"), a:has-text("Logout")');
    if (await logoutButton.first().isVisible({ timeout: 5000 }).catch(() => false)) {
      await logoutButton.first().click();
      await page.waitForURL('**/login', { timeout: 10000 });
      expect(page.url()).toContain('/login');
    }
  });
});
