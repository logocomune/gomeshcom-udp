import { expect, test } from '@playwright/test';

test('installs app shell and reopens a route offline', async ({ context, page, request }) => {
	await page.goto('/');

	await expect(page.locator('link[rel="manifest"]')).toHaveAttribute(
		'href',
		'/manifest.webmanifest'
	);
	const manifestResponse = await request.get('/manifest.webmanifest');
	expect(manifestResponse.ok()).toBe(true);
	const manifest = await manifestResponse.json();
	expect(manifest).toMatchObject({
		name: 'goMeshCom',
		short_name: 'goMeshCom',
		id: '/',
		start_url: '/',
		scope: '/',
		display: 'standalone',
		background_color: '#0f1729',
		theme_color: '#0f1729'
	});
	expect(manifest.icons).toEqual(
		expect.arrayContaining([
			expect.objectContaining({ sizes: '192x192', type: 'image/png' }),
			expect.objectContaining({ sizes: '512x512', type: 'image/png', purpose: 'any' }),
			expect.objectContaining({ sizes: '512x512', type: 'image/png', purpose: 'maskable' })
		])
	);
	for (const icon of manifest.icons) {
		expect((await request.get(icon.src)).ok()).toBe(true);
	}

	await page.evaluate(() => navigator.serviceWorker.ready);
	await page.reload();
	await expect
		.poll(() => page.evaluate(() => navigator.serviceWorker.controller !== null))
		.toBe(true);

	const cachedApiRequests = await page.evaluate(async () => {
		const cacheNames = await caches.keys();
		const requests = await Promise.all(
			cacheNames.map((cacheName) => caches.open(cacheName).then((cache) => cache.keys()))
		);
		return requests
			.flat()
			.filter((cachedRequest) => new URL(cachedRequest.url).pathname.startsWith('/api/')).length;
	});
	expect(cachedApiRequests).toBe(0);

	await context.setOffline(true);
	try {
		await page.goto('/about');
		await expect(page.getByRole('heading', { level: 1, name: 'goMeshCom' })).toBeVisible();
		const apiResult = await page.evaluate(() =>
			fetch('/api/health')
				.then(() => 'resolved')
				.catch(() => 'failed')
		);
		expect(apiResult).toBe('failed');
	} finally {
		await context.setOffline(false);
	}
});
