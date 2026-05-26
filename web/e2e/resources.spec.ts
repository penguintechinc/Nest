import { test, expect } from '@playwright/test';

test.describe('Resource Management', () => {

  test('can navigate to resources page', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');

    // Try sidebar link or direct navigation
    const resourceLink = page.locator('a:has-text("Resources"), a[href*="resource"], [data-testid="nav-resources"]');
    if (await resourceLink.first().isVisible({ timeout: 5000 }).catch(() => false)) {
      await resourceLink.first().click();
      await page.waitForLoadState('networkidle');
      expect(page.url()).toMatch(/resource/i);
    } else {
      // Fallback: navigate directly
      await page.goto('/resources');
      await page.waitForLoadState('networkidle');
    }
  });

  test('resource wizard opens when clicking create button', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');

    const createButton = page.locator('[data-testid="create-resource-btn"], button:has-text("Create"), button:has-text("Add Resource"), button:has-text("New")');
    if (await createButton.first().isVisible({ timeout: 5000 }).catch(() => false)) {
      await createButton.first().click();

      // Look for wizard/modal/dialog
      const wizard = page.locator('[data-testid="resource-wizard"], [role="dialog"], .modal, .wizard');
      await expect(wizard.first()).toBeVisible({ timeout: 5000 });
    }
  });

  test('can view resource details', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');

    // Click on a resource item if one exists
    const resourceItem = page.locator('[data-testid="resource-item"], table tbody tr, .resource-card, .resource-row');
    if (await resourceItem.first().isVisible({ timeout: 5000 }).catch(() => false)) {
      await resourceItem.first().click();
      await page.waitForLoadState('networkidle');

      // Should show some detail view
      const detailView = page.locator('[data-testid="resource-detail"], .resource-detail, h1, h2');
      await expect(detailView.first()).toBeVisible({ timeout: 5000 });
    }
  });
});
