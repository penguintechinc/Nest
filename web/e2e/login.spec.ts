import { test, expect, type Page } from '@playwright/test';

/**
 * The login form is intentionally disabled until cookie consent is given, so
 * any spec that types into it must accept consent first.
 */
async function acceptCookies(page: Page) {
  const accept = page.getByRole('button', { name: /accept all/i });
  await accept.click();
  await expect(page.locator('#tenant')).toBeEnabled();
}

test.describe('Login Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
  });

  test('shows the GDPR consent banner on first visit', async ({ page }) => {
    await expect(page.getByRole('button', { name: /accept all/i })).toBeVisible();
  });

  test('renders tenant, email, password, and submit button', async ({ page }) => {
    await expect(page.locator('#tenant')).toBeVisible();
    await expect(page.locator('#email')).toBeVisible();
    await expect(page.locator('#password')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('tenant field defaults so users need not know their tenant', async ({ page }) => {
    await expect(page.locator('#tenant')).toHaveValue('nest');
  });

  test('does not ask users for an API key', async ({ page }) => {
    await expect(page.locator('#token')).toHaveCount(0);
    await expect(page.getByLabel(/api (key|token)/i)).toHaveCount(0);
  });

  test('form is gated until cookie consent is given', async ({ page }) => {
    await expect(page.locator('#tenant')).toBeDisabled();
    await acceptCookies(page);
    await expect(page.locator('#tenant')).toBeEnabled();
  });

  test('blocks submit when credentials are empty', async ({ page }) => {
    await acceptCookies(page);

    await page.fill('#email', '');
    await page.fill('#password', '');
    await page.click('button[type="submit"]');

    // Fields are `required`, so the browser blocks submission and we stay on /login.
    await expect(page).toHaveURL(/\/login/);
    const emailValid = await page
      .locator('#email')
      .evaluate((el) => (el as HTMLInputElement).validity.valid);
    expect(emailValid).toBe(false);
  });

  test('login form accepts input and submit', async ({ page }) => {
    await acceptCookies(page);

    await page.fill('#tenant', 'acme');
    await page.fill('#email', 'user@example.com');
    await page.fill('#password', 'test-password-12345');

    expect(await page.inputValue('#tenant')).toBe('acme');
    expect(await page.inputValue('#email')).toBe('user@example.com');
    expect(await page.inputValue('#password')).toBe('test-password-12345');

    await expect(page.locator('button[type="submit"]')).toBeEnabled();
  });
});
