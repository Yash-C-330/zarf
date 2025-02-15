import { test, expect } from '@playwright/test';

test.beforeEach(async ({ page }) => {
	page.on('pageerror', (err) => console.log(err.message));
});

test.describe('start page', () => {
	test('spinner loads properly, then displays init btn @pre-init', async ({ page }) => {
		await page.goto('/auth?token=insecure');

		const clusterSelector = page.locator('#cluster-selector');
		await expect(clusterSelector).toBeEmpty();

		// display loading spinner
		const spinner = page.locator('.spinner');
		await expect(spinner).toBeVisible();

		// spinner disappears, init btn appears
		await expect(spinner).not.toBeVisible();

		// Make sure the home page contents are there
		await expect(page.locator('text=No Active Zarf Clusters')).toBeVisible();
		await expect(
			page.locator(
				'.hero-subtitle:has-text("cluster was found, click initialize cluster to initialize it now with Zarf")'
			)
		).toBeVisible();

		await page.locator('span:has-text("Initialize Cluster")').click();

		await page.waitForURL('**/initialize/configure');
	});
	test('page redirects to /packages @post-init', async ({ page }) => {
		await page.goto('/auth?token=insecure');

		// display loading spinner
		const spinner = page.locator('.spinner');
		await expect(spinner).toBeVisible();

		// spinner disappears
		await expect(spinner).not.toBeVisible();

		// expect to be redirected to /packages
		await page.waitForURL('/packages', { timeout: 10000 });
	});
});
