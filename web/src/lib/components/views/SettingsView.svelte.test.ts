import { page } from 'vitest/browser';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import type { AppConfig, ConfigFieldMeta } from '$lib/api/config';

const configAPI = vi.hoisted(() => ({
	getConfig: vi.fn(),
	updateConfig: vi.fn()
}));

vi.mock('$env/dynamic/public', () => ({ env: {} }));
vi.mock('$lib/api/config', async (importOriginal) => {
	const original = await importOriginal<typeof import('$lib/api/config')>();
	return {
		...original,
		getConfig: configAPI.getConfig,
		updateConfig: configAPI.updateConfig
	};
});

import SettingsView from './SettingsView.svelte';

describe('SettingsView serial transport', () => {
	beforeEach(() => {
		configAPI.getConfig.mockReset();
		configAPI.getConfig.mockResolvedValue(serialConfig());
	});

	it('shows serial parameters and board-specific modem-line guidance', async () => {
		expect.assertions(7);

		render(SettingsView);

		await expect.element(page.getByText('Serial device', { exact: true })).toBeVisible();
		await expect.element(page.getByPlaceholder('/dev/ttyUSB0 or COM3')).toHaveValue('/dev/ttyUSB0');
		await expect.element(page.getByText('Baud rate', { exact: true })).toBeVisible();
		await expect
			.element(
				page.getByText('Disable for ESP32/CP2102 to avoid reset; enable for nRF52/RAK USB CDC.')
			)
			.toBeVisible();
		expect(page.getByText('UDP listen address', { exact: true }).query()).toBeNull();

		const modemLines = document.querySelectorAll<HTMLInputElement>('input[type="checkbox"]');
		expect(modemLines[0]?.checked).toBe(true);
		expect(modemLines[1]?.checked).toBe(false);
	});
});

describe('SettingsView NETConsole transport', () => {
	beforeEach(() => {
		configAPI.getConfig.mockReset();
		configAPI.getConfig.mockResolvedValue(netConsoleConfig());
		configAPI.updateConfig.mockReset();
		configAPI.updateConfig.mockImplementation(async () => ({
			config: netConsoleConfig(),
			requires_restart: true
		}));
	});

	it('shows endpoint, masked password, timeouts, and plaintext warning', async () => {
		expect.assertions(6);

		render(SettingsView);

		await expect.element(page.getByText('NETConsole address', { exact: true })).toBeVisible();
		await expect
			.element(page.getByPlaceholder('192.168.1.53:2323'))
			.toHaveValue('meshcom.local:2323');
		await expect.element(page.getByText('NETConsole password', { exact: true })).toBeVisible();
		const password = document.querySelector<HTMLInputElement>('input[type="password"]');
		expect(password?.value).toBe('****');
		await expect.element(page.getByText(/NETConsole traffic is plaintext/)).toBeVisible();
		expect(page.getByText('Serial device', { exact: true }).query()).toBeNull();
	});

	it.each([
		{ name: 'clears', value: '' },
		{ name: 'replaces', value: 'new-password' }
	])('$name password and includes it in generated patch', async ({ value }) => {
		render(SettingsView);

		const password = page.getByPlaceholder('Open access');
		await expect.element(password).toHaveValue('****');
		await password.fill(value);
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(configAPI.updateConfig).toHaveBeenCalledOnce());
		const patch = configAPI.updateConfig.mock.calls[0]?.[0];
		expect(patch.netconsole.password).toBe(value);
		expect(patch.netconsole.address).toBe('meshcom.local:2323');
	});

	it('disables password changes when environment-managed', async () => {
		const config = netConsoleConfig();
		config.netconsole.password.env_override = true;
		configAPI.getConfig.mockResolvedValue(config);

		render(SettingsView);

		await expect.element(page.getByPlaceholder('Open access')).toBeDisabled();
	});
});

function field<T>(value: T): ConfigFieldMeta<T> {
	return { value, env_override: false, requires_restart: true };
}

function serialConfig(): AppConfig {
	return {
		server: { version: 'test', started_at: '2026-07-29T08:00:00Z', uptime_seconds: 1 },
		http_addr: field('127.0.0.1:8080'),
		transport_mode: field('serial'),
		udp_listen_addr: field('0.0.0.0:1799'),
		node_addr: field(''),
		my_call: field('QQ0QQ-1'),
		data_dir: field('./data'),
		max_message_length: field(149),
		log_level: field('info'),
		serial: {
			device: field('/dev/ttyUSB0'),
			baud: field(115200),
			data_bits: field(8),
			parity: field('none'),
			stop_bits: field(1),
			flow_control: field('none'),
			dtr: field(true),
			rts: field(false),
			read_timeout: field('1s'),
			reconnect_initial: field('1s'),
			reconnect_max: field('30s'),
			stable_reset_after: field('30s'),
			max_record_bytes: field(65536)
		},
		netconsole: {
			address: field('meshcom.local:2323'),
			password: field('****'),
			connect_timeout: field('5s'),
			auth_timeout: field('5s'),
			write_timeout: field('5s'),
			reconnect_initial: field('1s'),
			reconnect_max: field('30s'),
			stable_reset_after: field('30s'),
			max_auth_line_bytes: field(128),
			max_record_bytes: field(65536)
		},
		receive_log: {
			enabled: field(true),
			path: field('./data/raw'),
			retention_days: field(365),
			replay_window: field('1h0m0s')
		},
		stats: {
			enabled: field(true),
			path: field('./data/stats/stats.json'),
			retention_days: field(30)
		},
		chat_log: {
			path: field('./data/chat'),
			history_window: field('24h0m0s'),
			max_history_window: field('720h0m0s')
		},
		send: { dedup_ttl: field('2s') },
		forward: { targets: field('') },
		auth: {
			username: field(''),
			password: field(''),
			session_ttl: field('24h0m0s'),
			cookie_name: field('meshcom_session')
		},
		request_log: { enabled: field(false) },
		storage: {
			sqlite_path: field('./data/gomeshcom.db'),
			purge_interval: field('4h0m0s'),
			receive_log_retention: field('720h0m0s'),
			public_chat_retention: field('720h0m0s'),
			nodes_retention: field('168h0m0s'),
			telemetry_retention: field('720h0m0s')
		}
	};
}

function netConsoleConfig(): AppConfig {
	const config = serialConfig();
	config.transport_mode = field<'udp' | 'serial' | 'netconsole'>('netconsole');
	return config;
}
