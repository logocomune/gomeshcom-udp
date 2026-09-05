/// <reference no-default-lib="true" />
/// <reference lib="esnext" />
/// <reference lib="webworker" />
/// <reference types="@sveltejs/kit" />

import { build, files, prerendered, version } from '$service-worker';
import { cacheNamesToDelete, chooseRequestStrategy } from './lib/pwa/cache-policy';

const worker = globalThis as unknown as ServiceWorkerGlobalScope;
const CACHE_PREFIX = 'gomeshcom-';
const CACHE_NAME = `${CACHE_PREFIX}${version}`;
const OFFLINE_SHELL = '/';
const RUNTIME_ENVIRONMENT = '/_app/env.js';
const PRECACHE = [
	...new Set([OFFLINE_SHELL, RUNTIME_ENVIRONMENT, ...build, ...files, ...prerendered])
];
const PRECACHED_PATHS = new Set(
	PRECACHE.map((path) => new URL(path, worker.location.origin).pathname)
);

worker.addEventListener('install', (event) => {
	event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(PRECACHE)));
});

worker.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((cacheNames) =>
				Promise.all(
					cacheNamesToDelete({ cacheNames, currentCache: CACHE_NAME, prefix: CACHE_PREFIX }).map(
						(cacheName) => caches.delete(cacheName)
					)
				)
			)
	);
});

worker.addEventListener('fetch', (event) => {
	const strategy = chooseRequestStrategy({
		origin: worker.location.origin,
		precachedPaths: PRECACHED_PATHS,
		request: event.request
	});

	if (strategy === 'cache-first') {
		event.respondWith(cacheFirst(event.request));
		return;
	}
	if (strategy === 'navigation-network-first') {
		event.respondWith(navigationNetworkFirst(event.request));
	}
});

async function cacheFirst(request: Request): Promise<Response> {
	const cache = await caches.open(CACHE_NAME);
	return (await cache.match(request)) ?? fetch(request);
}

async function navigationNetworkFirst(request: Request): Promise<Response> {
	try {
		return await fetch(request);
	} catch {
		const cachedShell = await caches.match(OFFLINE_SHELL);
		if (cachedShell) return cachedShell;
		throw new Error('Offline shell is unavailable');
	}
}
