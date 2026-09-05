import fc from 'fast-check';
import { describe, expect, it } from 'vitest';
import { cacheNamesToDelete, chooseRequestStrategy } from './cache-policy';

const origin = 'https://mesh.example';
const precachedPaths = new Set(['/', '/_app/env.js', '/_app/immutable/app.js', '/pwa-192.png']);

describe('chooseRequestStrategy', () => {
	it.each([
		{
			name: 'ignores non-GET requests',
			request: { method: 'POST', mode: 'cors', url: `${origin}/messages` },
			want: 'ignore'
		},
		{
			name: 'ignores cross-origin requests',
			request: { method: 'GET', mode: 'cors', url: 'https://tiles.example/map.png' },
			want: 'ignore'
		},
		{
			name: 'keeps API requests network-only',
			request: { method: 'GET', mode: 'cors', url: `${origin}/api/health` },
			want: 'network-only'
		},
		{
			name: 'serves precached assets cache-first',
			request: { method: 'GET', mode: 'cors', url: `${origin}/_app/immutable/app.js` },
			want: 'cache-first'
		},
		{
			name: 'serves runtime environment cache-first',
			request: { method: 'GET', mode: 'cors', url: `${origin}/_app/env.js` },
			want: 'cache-first'
		},
		{
			name: 'uses network-first for navigation',
			request: { method: 'GET', mode: 'navigate', url: `${origin}/about` },
			want: 'navigation-network-first'
		},
		{
			name: 'uses network-first for a precached navigation',
			request: { method: 'GET', mode: 'navigate', url: `${origin}/` },
			want: 'navigation-network-first'
		},
		{
			name: 'keeps unknown same-origin resources network-only',
			request: { method: 'GET', mode: 'cors', url: `${origin}/dynamic.json` },
			want: 'network-only'
		}
	])('$name', ({ request, want }) => {
		expect(chooseRequestStrategy({ origin, precachedPaths, request })).toBe(want);
	});

	it('never caches API paths', () => {
		fc.assert(
			fc.property(fc.string(), (suffix) => {
				const url = new URL(`/api/${encodeURIComponent(suffix)}`, origin).toString();
				const strategy = chooseRequestStrategy({
					origin,
					precachedPaths: new Set([new URL(url).pathname]),
					request: { method: 'GET', mode: 'navigate', url }
				});
				expect(strategy).toBe('network-only');
			})
		);
	});
});

describe('cacheNamesToDelete', () => {
	it('deletes only obsolete goMeshCom caches', () => {
		expect(
			cacheNamesToDelete({
				cacheNames: ['gomeshcom-old', 'gomeshcom-current', 'unrelated-cache'],
				currentCache: 'gomeshcom-current',
				prefix: 'gomeshcom-'
			})
		).toEqual(['gomeshcom-old']);
	});
});
