export type RequestStrategy =
	| 'ignore'
	| 'cache-first'
	| 'navigation-network-first'
	| 'network-only';

type RequestDescription = {
	method: string;
	mode: string;
	url: string;
};

type RequestPolicyInput = {
	origin: string;
	precachedPaths: ReadonlySet<string>;
	request: RequestDescription;
};

type CacheCleanupInput = {
	cacheNames: readonly string[];
	currentCache: string;
	prefix: string;
};

export function chooseRequestStrategy(input: RequestPolicyInput): RequestStrategy {
	if (input.request.method !== 'GET') return 'ignore';

	const url = new URL(input.request.url);
	if (url.origin !== input.origin) return 'ignore';
	if (url.pathname === '/api' || url.pathname.startsWith('/api/')) return 'network-only';
	if (input.request.mode === 'navigate') return 'navigation-network-first';
	if (input.precachedPaths.has(url.pathname)) return 'cache-first';
	return 'network-only';
}

export function cacheNamesToDelete(input: CacheCleanupInput): string[] {
	return input.cacheNames.filter(
		(cacheName) => cacheName.startsWith(input.prefix) && cacheName !== input.currentCache
	);
}
