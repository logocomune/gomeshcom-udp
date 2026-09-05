import { page } from 'vitest/browser';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { vi } from 'vitest';

vi.mock('$env/dynamic/public', () => ({ env: {} }));

import TransportWarning from './TransportWarning.svelte';

describe('TransportWarning', () => {
	it('shows serial failure without changing API connection state', async () => {
		expect.assertions(3);

		render(TransportWarning, {
			props: { transport: { mode: 'serial', state: 'degraded', last_error: 'device lost' } }
		});

		await expect.element(page.getByText('Serial unavailable')).toBeVisible();
		await expect.element(page.getByText('device lost')).toBeVisible();
		await expect.element(page.getByTestId('transport-warning')).toHaveClass('max-w-28');
	});

	it.each(['connecting', 'degraded', 'disconnected', 'stopped'])(
		'shows NETConsole failure in %s state',
		async (state) => {
			render(TransportWarning, {
				props: {
					transport: {
						mode: 'netconsole',
						state,
						last_error: 'authentication rejected'
					}
				}
			});
			await expect.element(page.getByText('NETConsole unavailable')).toBeVisible();
			await expect.element(page.getByText('authentication rejected')).toBeVisible();
		}
	);

	it('hides warning for connected serial and all UDP states', () => {
		render(TransportWarning, { props: { transport: { mode: 'serial', state: 'connected' } } });
		expect(page.getByText('Serial unavailable').query()).toBeNull();

		render(TransportWarning, { props: { transport: { mode: 'udp', state: 'degraded' } } });
		expect(page.getByText('Serial unavailable').query()).toBeNull();

		render(TransportWarning, {
			props: { transport: { mode: 'netconsole', state: 'connected' } }
		});
		expect(page.getByText('NETConsole unavailable').query()).toBeNull();
	});
});
