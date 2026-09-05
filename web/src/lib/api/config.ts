import { API_BASE } from './events';
import { apiFetch } from './auth';

export type ConfigFieldMeta<T> = {
	value: T;
	env_override: boolean;
	requires_restart: boolean;
};

export type ConfigReceiveLog = {
	enabled: ConfigFieldMeta<boolean>;
	path: ConfigFieldMeta<string>;
	retention_days: ConfigFieldMeta<number>;
	replay_window: ConfigFieldMeta<string>;
};

export type ConfigStats = {
	enabled: ConfigFieldMeta<boolean>;
	path: ConfigFieldMeta<string>;
	retention_days: ConfigFieldMeta<number>;
};

export type ConfigChatLog = {
	path: ConfigFieldMeta<string>;
	history_window: ConfigFieldMeta<string>;
	max_history_window: ConfigFieldMeta<string>;
};

export type ConfigSend = {
	dedup_ttl: ConfigFieldMeta<string>;
};

export type ConfigForward = {
	targets: ConfigFieldMeta<string>;
};

export type ConfigAuth = {
	username: ConfigFieldMeta<string>;
	password: ConfigFieldMeta<string>; // value is always masked by server
	session_ttl: ConfigFieldMeta<string>;
	cookie_name: ConfigFieldMeta<string>;
};

export type ConfigRequestLog = {
	enabled: ConfigFieldMeta<boolean>;
};

export type ConfigStorage = {
	sqlite_path: ConfigFieldMeta<string>;
	purge_interval: ConfigFieldMeta<string>;
	receive_log_retention: ConfigFieldMeta<string>;
	public_chat_retention: ConfigFieldMeta<string>;
	nodes_retention: ConfigFieldMeta<string>;
	telemetry_retention: ConfigFieldMeta<string>;
};

export type ConfigSerial = {
	device: ConfigFieldMeta<string>;
	baud: ConfigFieldMeta<number>;
	data_bits: ConfigFieldMeta<number>;
	parity: ConfigFieldMeta<string>;
	stop_bits: ConfigFieldMeta<number>;
	flow_control: ConfigFieldMeta<string>;
	dtr: ConfigFieldMeta<boolean>;
	rts: ConfigFieldMeta<boolean>;
	read_timeout: ConfigFieldMeta<string>;
	reconnect_initial: ConfigFieldMeta<string>;
	reconnect_max: ConfigFieldMeta<string>;
	stable_reset_after: ConfigFieldMeta<string>;
	max_record_bytes: ConfigFieldMeta<number>;
};

export type ConfigNetConsole = {
	address: ConfigFieldMeta<string>;
	password: ConfigFieldMeta<string>;
	connect_timeout: ConfigFieldMeta<string>;
	auth_timeout: ConfigFieldMeta<string>;
	write_timeout: ConfigFieldMeta<string>;
	reconnect_initial: ConfigFieldMeta<string>;
	reconnect_max: ConfigFieldMeta<string>;
	stable_reset_after: ConfigFieldMeta<string>;
	max_auth_line_bytes: ConfigFieldMeta<number>;
	max_record_bytes: ConfigFieldMeta<number>;
};

export type ServerInfo = {
	version: string;
	started_at: string;
	uptime_seconds: number;
};

export type AppConfig = {
	server: ServerInfo;
	http_addr: ConfigFieldMeta<string>;
	transport_mode: ConfigFieldMeta<'udp' | 'serial' | 'netconsole'>;
	udp_listen_addr: ConfigFieldMeta<string>;
	node_addr: ConfigFieldMeta<string>;
	my_call: ConfigFieldMeta<string>;
	data_dir: ConfigFieldMeta<string>;
	max_message_length: ConfigFieldMeta<number>;
	log_level: ConfigFieldMeta<string>;
	receive_log: ConfigReceiveLog;
	stats: ConfigStats;
	chat_log: ConfigChatLog;
	send: ConfigSend;
	forward: ConfigForward;
	auth: ConfigAuth;
	request_log: ConfigRequestLog;
	storage: ConfigStorage;
	serial: ConfigSerial;
	netconsole: ConfigNetConsole;
};

export type ConfigUpdateResponse = {
	config: AppConfig;
	requires_restart: boolean;
};

export type ConfigPatch = {
	http_addr?: string;
	transport_mode?: 'udp' | 'serial' | 'netconsole';
	udp_listen_addr?: string;
	node_addr?: string;
	my_call?: string;
	max_message_length?: number;
	log_level?: string;
	receive_log?: {
		enabled?: boolean;
		path?: string;
		retention_days?: number;
		replay_window?: string;
	};
	stats?: {
		enabled?: boolean;
		path?: string;
		retention_days?: number;
	};
	chat_log?: {
		path?: string;
		history_window?: string;
		max_history_window?: string;
	};
	send?: {
		dedup_ttl?: string;
	};
	forward?: {
		targets?: string;
	};
	auth?: {
		username?: string;
		password?: string;
		session_ttl?: string;
		cookie_name?: string;
	};
	request_log?: {
		enabled?: boolean;
	};
	storage?: {
		sqlite_path?: string;
		purge_interval?: string;
		receive_log_retention?: string;
		public_chat_retention?: string;
		nodes_retention?: string;
		telemetry_retention?: string;
	};
	serial?: {
		device?: string;
		baud?: number;
		data_bits?: number;
		parity?: string;
		stop_bits?: number;
		flow_control?: string;
		dtr?: boolean;
		rts?: boolean;
		read_timeout?: string;
		reconnect_initial?: string;
		reconnect_max?: string;
		stable_reset_after?: string;
		max_record_bytes?: number;
	};
	netconsole?: {
		address?: string;
		password?: string;
		connect_timeout?: string;
		auth_timeout?: string;
		write_timeout?: string;
		reconnect_initial?: string;
		reconnect_max?: string;
		stable_reset_after?: string;
		max_auth_line_bytes?: number;
		max_record_bytes?: number;
	};
};

export class DemoModeError extends Error {
	constructor() {
		super('config API disabled in demo mode');
		this.name = 'DemoModeError';
	}
}

export async function getConfig(): Promise<AppConfig> {
	const res = await apiFetch(`${API_BASE}/config`);
	if (res.status === 403) throw new DemoModeError();
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`getConfig failed: ${res.status}${text ? ` — ${text}` : ''}`);
	}
	return (await res.json()) as AppConfig;
}

export async function getStartedAt(): Promise<string | null> {
	try {
		const r = await fetch(`${API_BASE}/health`, { cache: 'no-store' });
		if (!r.ok) return null;
		const body = await r.json();
		return (body as Record<string, string>).started_at ?? null;
	} catch {
		return null;
	}
}

export async function shutdownApp(): Promise<void> {
	const res = await apiFetch(`${API_BASE}/shutdown`, { method: 'POST' });
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`shutdown failed: ${res.status}${text ? ` — ${text}` : ''}`);
	}
}

export async function restartApp(): Promise<void> {
	const res = await apiFetch(`${API_BASE}/restart`, { method: 'POST' });
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`restart failed: ${res.status}${text ? ` — ${text}` : ''}`);
	}
}

// waitForRestart polls /api/health until started_at differs from the value
// recorded before the restart was triggered. Rejects after timeoutMs.
export async function waitForRestart(
	previousStartedAt: string | null,
	timeoutMs = 30_000
): Promise<void> {
	const pollInterval = 500;
	const deadline = Date.now() + timeoutMs;

	while (Date.now() < deadline) {
		try {
			const r = await fetch(`${API_BASE}/health`, { cache: 'no-store' });
			if (r.ok) {
				const body = await r.json();
				const current = (body as Record<string, string>).started_at;
				if (current && current !== previousStartedAt) return;
			}
		} catch {
			// server still down
		}
		await new Promise((r) => setTimeout(r, pollInterval));
	}

	throw new Error('server did not come back up within timeout');
}

export async function updateConfig(patch: ConfigPatch): Promise<ConfigUpdateResponse> {
	const res = await apiFetch(`${API_BASE}/config`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(patch)
	});
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`updateConfig failed: ${res.status}${text ? ` — ${text}` : ''}`);
	}
	return (await res.json()) as ConfigUpdateResponse;
}
