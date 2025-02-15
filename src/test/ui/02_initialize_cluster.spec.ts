import { expect, test } from '@playwright/test';

test.beforeEach(async ({ page }) => {
	page.on('pageerror', (err) => console.log(err.message));
});

test.describe('initialize a zarf cluster', () => {
	test('configure the init package @pre-init', async ({ page }) => {
		await page.goto('/auth?token=insecure&next=/initialize/configure');

		// Stepper
		const stepperItems = await page.locator('.stepper .stepper-item .step');
		await expect(stepperItems.nth(0).locator('.step-icon')).toHaveClass(/primary/);
		await expect(stepperItems.nth(0)).toContainText('Configure');
		await expect(stepperItems.nth(1).locator('.step-icon')).toHaveClass(/primary/);
		await expect(stepperItems.nth(1)).toContainText('2 Review');
		await expect(stepperItems.nth(2).locator('.step-icon')).toHaveClass(/disabled/);
		await expect(stepperItems.nth(2)).toContainText('3 Deploy');

		// Package details
		await expect(page.locator('text=Package Type ZarfInitConfig')).toBeVisible();
		await expect(
			page.locator('text=METADATA Name: Init Description: Used to establish a new Zarf cluster')
		).toBeVisible();

		// Components (check most functionaliy with k3s component)
		let k3s = page.locator('.accordion:has-text("k3s (Optional)")');
		await expect(k3s.locator('.deploy-component-toggle')).toHaveAttribute('aria-pressed', 'false');
		await k3s.locator('text=Deploy').click();
		await expect(k3s.locator('.deploy-component-toggle')).toHaveAttribute('aria-pressed', 'true');
		await expect(
			page.locator('.component-accordion-header:has-text("*** REQUIRES ROOT *** Install K3s")')
		).toBeVisible();
		await expect(k3s.locator('code')).toBeHidden();
		await k3s.locator('.accordion-toggle').click();
		await expect(k3s.locator('code')).toBeVisible();
		await expect(k3s.locator('code:has-text("name: k3s")')).toBeVisible();

		// Check remaining components for deploy states
		await validateRequiredCheckboxes(page);

		let loggingDeployToggle = page
			.locator('.accordion:has-text("logging (Optional)")')
			.locator('.deploy-component-toggle');
		await loggingDeployToggle.click();
		await expect(loggingDeployToggle).toHaveAttribute('aria-pressed', 'true');

		let gitServerDeployToggle = page
			.locator('.accordion:has-text("git-server (Optional)")')
			.locator('.deploy-component-toggle');
		await gitServerDeployToggle.click();
		await expect(gitServerDeployToggle).toHaveAttribute('aria-pressed', 'true');

		await page.locator('text=review deployment').click();
		await expect(page).toHaveURL('/initialize/review');
	});

	test('review the init package @pre-init', async ({ page }) => {
		await page.goto('/auth?token=insecure&next=/initialize/review');

		await validateRequiredCheckboxes(page);
	});

	test('deploy the init package @init', async ({ page }) => {
		await page.goto('/auth?token=insecure&next=/');
		await page.getByRole('link', { name: 'Initialize Cluster' }).click();
		await page.waitForURL('/initialize/configure');
		await page.getByRole('link', { name: 'review deployment' }).click();
		await page.waitForURL('/initialize/review');
		await page.getByRole('link', { name: 'deploy' }).click();
		await page.waitForURL('/initialize/deploy');

		// expect all steps to have success class
		const stepperItems = page.locator('.stepper-vertical .step-icon');

		// deploy zarf-injector
		await expect(stepperItems.nth(0)).toHaveClass(/success/, {
			timeout: 45000
		});
		// deploy zarf-seed-registry
		await expect(stepperItems.nth(1)).toHaveClass(/success/, {
			timeout: 45000
		});
		// deploy zarf-registry
		await expect(stepperItems.nth(2)).toHaveClass(/success/, {
			timeout: 45000
		});
		// deploy zarf-agent
		await expect(stepperItems.nth(3)).toHaveClass(/success/, {
			timeout: 45000
		});

		// verify the final step succeeded
		await expect(page.locator('text=Deployment Succeeded')).toBeVisible();

		// then verify the page redirects to the packages dashboard
		await page.waitForURL('/packages', { timeout: 10000 });
	});
});

async function validateRequiredCheckboxes(page) {
	// Check remaining components for deploy states
	const injector = page.locator('.accordion:has-text("zarf-injector (Required)")');
	await expect(injector.locator('.deploy-component-toggle')).toBeHidden();

	const seedRegistry = page.locator('.accordion:has-text("zarf-seed-registry (Required)")');
	await expect(seedRegistry.locator('.deploy-component-toggle')).toBeHidden();

	const registry = page.locator('.accordion:has-text("zarf-registry (Required)")');
	await expect(registry.locator('.deploy-component-toggle')).toBeHidden();
}
