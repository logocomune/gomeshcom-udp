<script lang="ts">
	import { onMount } from 'svelte';
	import MdiIcon from '$lib/components/MdiIcon.svelte';
	import SettingsField from '$lib/components/settings/SettingsField.svelte';
	import SettingsInfoRow from '$lib/components/settings/SettingsInfoRow.svelte';
	import { isValidCallsign, normalizeCallsign } from '$lib/ui/callsign';
	import {
		getConfig,
		updateConfig,
		restartApp,
		shutdownApp,
		waitForRestart,
		getStartedAt,
		DemoModeError
	} from '$lib/api/config';
	import type { AppConfig, ConfigPatch } from '$lib/api/config';
	import { connectionState } from '$lib/stores/connection.svelte';
	import {
		mdiTune,
		mdiSend,
		mdiDownload,
		mdiSwapVertical,
		mdiChartBar,
		mdiMessageTextOutline,
		mdiArrowRight,
		mdiInformationOutline
	} from '@mdi/js';

	function formatUptime(seconds: number): string {
		if (seconds <= 0) return '—';
		const d = Math.floor(seconds / 86400);
		const h = Math.floor((seconds % 86400) / 3600);
		const m = Math.floor((seconds % 3600) / 60);
		const s = seconds % 60;
		const parts: string[] = [];
		if (d > 0) parts.push(`${d}d`);
		if (h > 0) parts.push(`${h}h`);
		if (m > 0) parts.push(`${m}m`);
		if (parts.length === 0 || (d === 0 && h === 0)) parts.push(`${s}s`);
		return parts.join(' ');
	}

	function formatStartedAt(iso: string): string {
		if (!iso || iso.startsWith('0001')) return '—';
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	let config = $state<AppConfig | null>(null);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	let demoMode = $state(false);

	let saveError = $state<string | null>(null);
	let saveSuccess = $state(false);
	let requiresRestart = $state(false);
	let saving = $state(false);
	let restarting = $state(false);
	let restartError = $state<string | null>(null);
	let shuttingDown = $state(false);
	let shutdownError = $state<string | null>(null);

	type Section = 'server' | 'web' | 'storage' | 'system';
	let activeSection = $state<Section>('server');

	// Core
	let myCall = $state('');
	let logLevel = $state('info');
	let httpAddr = $state('');
	let transportMode = $state<'udp' | 'serial' | 'netconsole'>('udp');
	let udpListenAddr = $state('');
	let nodeAddr = $state('');
	let maxMsgLen = $state('');
	// Serial transport
	let serialDevice = $state('');
	let serialBaud = $state('');
	let serialDataBits = $state('');
	let serialParity = $state('none');
	let serialStopBits = $state('');
	let serialFlowControl = $state('none');
	let serialDtr = $state(false);
	let serialRts = $state(false);
	let serialReadTimeout = $state('');
	let serialReconnectInitial = $state('');
	let serialReconnectMax = $state('');
	let serialStableResetAfter = $state('');
	let serialMaxRecordBytes = $state('');
	// NETConsole transport
	let netConsoleAddress = $state('');
	let netConsolePassword = $state('');
	let netConsoleConnectTimeout = $state('');
	let netConsoleAuthTimeout = $state('');
	let netConsoleWriteTimeout = $state('');
	let netConsoleReconnectInitial = $state('');
	let netConsoleReconnectMax = $state('');
	let netConsoleStableResetAfter = $state('');
	let netConsoleMaxAuthLineBytes = $state('');
	let netConsoleMaxRecordBytes = $state('');
	// Send
	let dedupTtl = $state('');
	// Receive Log
	let receiveLogEnabled = $state(true);
	let receiveLogPath = $state('');
	let receiveLogRetentionDays = $state('');
	let receiveLogReplayWindow = $state('');
	// Request Log
	let requestLogEnabled = $state(false);
	// Stats
	let statsEnabled = $state(true);
	let statsPath = $state('');
	let statsRetentionDays = $state('');
	// Chat Log
	let chatLogPath = $state('');
	let chatLogHistoryWindow = $state('');
	let chatLogMaxHistoryWindow = $state('');

	onMount(async () => {
		await reload();
	});

	async function reload() {
		loading = true;
		loadError = null;
		demoMode = false;
		try {
			config = await getConfig();
			populate(config);
		} catch (err) {
			if (err instanceof DemoModeError) {
				demoMode = true;
			} else {
				loadError = err instanceof Error ? err.message : 'Failed to load config';
			}
		} finally {
			loading = false;
		}
	}

	let isDirty = $derived(
		config !== null &&
			(myCall !== config.my_call.value ||
				logLevel !== config.log_level.value ||
				httpAddr !== config.http_addr.value ||
				transportMode !== config.transport_mode.value ||
				udpListenAddr !== config.udp_listen_addr.value ||
				nodeAddr !== config.node_addr.value ||
				maxMsgLen !== String(config.max_message_length.value) ||
				serialDevice !== config.serial.device.value ||
				serialBaud !== String(config.serial.baud.value) ||
				serialDataBits !== String(config.serial.data_bits.value) ||
				serialParity !== config.serial.parity.value ||
				serialStopBits !== String(config.serial.stop_bits.value) ||
				serialFlowControl !== config.serial.flow_control.value ||
				serialDtr !== config.serial.dtr.value ||
				serialRts !== config.serial.rts.value ||
				serialReadTimeout !== config.serial.read_timeout.value ||
				serialReconnectInitial !== config.serial.reconnect_initial.value ||
				serialReconnectMax !== config.serial.reconnect_max.value ||
				serialStableResetAfter !== config.serial.stable_reset_after.value ||
				serialMaxRecordBytes !== String(config.serial.max_record_bytes.value) ||
				netConsoleAddress !== config.netconsole.address.value ||
				netConsolePassword !== config.netconsole.password.value ||
				netConsoleConnectTimeout !== config.netconsole.connect_timeout.value ||
				netConsoleAuthTimeout !== config.netconsole.auth_timeout.value ||
				netConsoleWriteTimeout !== config.netconsole.write_timeout.value ||
				netConsoleReconnectInitial !== config.netconsole.reconnect_initial.value ||
				netConsoleReconnectMax !== config.netconsole.reconnect_max.value ||
				netConsoleStableResetAfter !== config.netconsole.stable_reset_after.value ||
				netConsoleMaxAuthLineBytes !== String(config.netconsole.max_auth_line_bytes.value) ||
				netConsoleMaxRecordBytes !== String(config.netconsole.max_record_bytes.value) ||
				dedupTtl !== config.send.dedup_ttl.value ||
				receiveLogEnabled !== config.receive_log.enabled.value ||
				receiveLogPath !== config.receive_log.path.value ||
				receiveLogRetentionDays !== String(config.receive_log.retention_days.value) ||
				receiveLogReplayWindow !== config.receive_log.replay_window.value ||
				requestLogEnabled !== config.request_log.enabled.value ||
				statsEnabled !== config.stats.enabled.value ||
				statsPath !== config.stats.path.value ||
				statsRetentionDays !== String(config.stats.retention_days.value) ||
				chatLogPath !== config.chat_log.path.value ||
				chatLogHistoryWindow !== config.chat_log.history_window.value ||
				chatLogMaxHistoryWindow !== config.chat_log.max_history_window.value)
	);

	function populate(cfg: AppConfig) {
		myCall = cfg.my_call.value;
		logLevel = cfg.log_level.value;
		httpAddr = cfg.http_addr.value;
		transportMode = cfg.transport_mode.value;
		udpListenAddr = cfg.udp_listen_addr.value;
		nodeAddr = cfg.node_addr.value;
		maxMsgLen = String(cfg.max_message_length.value);
		serialDevice = cfg.serial.device.value;
		serialBaud = String(cfg.serial.baud.value);
		serialDataBits = String(cfg.serial.data_bits.value);
		serialParity = cfg.serial.parity.value;
		serialStopBits = String(cfg.serial.stop_bits.value);
		serialFlowControl = cfg.serial.flow_control.value;
		serialDtr = cfg.serial.dtr.value;
		serialRts = cfg.serial.rts.value;
		serialReadTimeout = cfg.serial.read_timeout.value;
		serialReconnectInitial = cfg.serial.reconnect_initial.value;
		serialReconnectMax = cfg.serial.reconnect_max.value;
		serialStableResetAfter = cfg.serial.stable_reset_after.value;
		serialMaxRecordBytes = String(cfg.serial.max_record_bytes.value);
		netConsoleAddress = cfg.netconsole.address.value;
		netConsolePassword = cfg.netconsole.password.value;
		netConsoleConnectTimeout = cfg.netconsole.connect_timeout.value;
		netConsoleAuthTimeout = cfg.netconsole.auth_timeout.value;
		netConsoleWriteTimeout = cfg.netconsole.write_timeout.value;
		netConsoleReconnectInitial = cfg.netconsole.reconnect_initial.value;
		netConsoleReconnectMax = cfg.netconsole.reconnect_max.value;
		netConsoleStableResetAfter = cfg.netconsole.stable_reset_after.value;
		netConsoleMaxAuthLineBytes = String(cfg.netconsole.max_auth_line_bytes.value);
		netConsoleMaxRecordBytes = String(cfg.netconsole.max_record_bytes.value);
		dedupTtl = cfg.send.dedup_ttl.value;
		receiveLogEnabled = cfg.receive_log.enabled.value;
		receiveLogPath = cfg.receive_log.path.value;
		receiveLogRetentionDays = String(cfg.receive_log.retention_days.value);
		receiveLogReplayWindow = cfg.receive_log.replay_window.value;
		requestLogEnabled = cfg.request_log.enabled.value;
		statsEnabled = cfg.stats.enabled.value;
		statsPath = cfg.stats.path.value;
		statsRetentionDays = String(cfg.stats.retention_days.value);
		chatLogPath = cfg.chat_log.path.value;
		chatLogHistoryWindow = cfg.chat_log.history_window.value;
		chatLogMaxHistoryWindow = cfg.chat_log.max_history_window.value;
	}

	async function save() {
		if (!config) return;
		saving = true;
		saveError = null;
		saveSuccess = false;
		requiresRestart = false;

		const normalizedCall = normalizeCallsign(myCall);
		if (!isValidCallsign(normalizedCall)) {
			saveError =
				'Invalid callsign — use 3-10 alphanumeric characters with optional numeric SSID (e.g. IU5PMP-1)';
			saving = false;
			return;
		}

		const maxLen = parseInt(maxMsgLen, 10);
		if (isNaN(maxLen) || maxLen <= 0) {
			saveError = 'Max message length must be a positive integer';
			saving = false;
			return;
		}

		const patch: ConfigPatch = {};

		if (!config.my_call.env_override) patch.my_call = normalizedCall;
		if (!config.log_level.env_override) patch.log_level = logLevel;
		if (!config.http_addr.env_override) patch.http_addr = httpAddr;
		if (!config.transport_mode.env_override) patch.transport_mode = transportMode;
		if (!config.udp_listen_addr.env_override) patch.udp_listen_addr = udpListenAddr;
		if (!config.node_addr.env_override) patch.node_addr = nodeAddr;
		if (!config.max_message_length.env_override) patch.max_message_length = maxLen;

		{
			const serial = config.serial;
			patch.serial = {};
			if (!serial.device.env_override) patch.serial.device = serialDevice;
			if (!serial.baud.env_override) patch.serial.baud = parseInt(serialBaud, 10);
			if (!serial.data_bits.env_override) patch.serial.data_bits = parseInt(serialDataBits, 10);
			if (!serial.parity.env_override) patch.serial.parity = serialParity;
			if (!serial.stop_bits.env_override) patch.serial.stop_bits = parseInt(serialStopBits, 10);
			if (!serial.flow_control.env_override) patch.serial.flow_control = serialFlowControl;
			if (!serial.dtr.env_override) patch.serial.dtr = serialDtr;
			if (!serial.rts.env_override) patch.serial.rts = serialRts;
			if (!serial.read_timeout.env_override) patch.serial.read_timeout = serialReadTimeout;
			if (!serial.reconnect_initial.env_override)
				patch.serial.reconnect_initial = serialReconnectInitial;
			if (!serial.reconnect_max.env_override) patch.serial.reconnect_max = serialReconnectMax;
			if (!serial.stable_reset_after.env_override)
				patch.serial.stable_reset_after = serialStableResetAfter;
			if (!serial.max_record_bytes.env_override)
				patch.serial.max_record_bytes = parseInt(serialMaxRecordBytes, 10);
			if (Object.keys(patch.serial).length === 0) delete patch.serial;
		}

		{
			const netconsole = config.netconsole;
			patch.netconsole = {};
			if (!netconsole.address.env_override) patch.netconsole.address = netConsoleAddress;
			if (!netconsole.password.env_override) patch.netconsole.password = netConsolePassword;
			if (!netconsole.connect_timeout.env_override)
				patch.netconsole.connect_timeout = netConsoleConnectTimeout;
			if (!netconsole.auth_timeout.env_override)
				patch.netconsole.auth_timeout = netConsoleAuthTimeout;
			if (!netconsole.write_timeout.env_override)
				patch.netconsole.write_timeout = netConsoleWriteTimeout;
			if (!netconsole.reconnect_initial.env_override)
				patch.netconsole.reconnect_initial = netConsoleReconnectInitial;
			if (!netconsole.reconnect_max.env_override)
				patch.netconsole.reconnect_max = netConsoleReconnectMax;
			if (!netconsole.stable_reset_after.env_override)
				patch.netconsole.stable_reset_after = netConsoleStableResetAfter;
			if (!netconsole.max_auth_line_bytes.env_override)
				patch.netconsole.max_auth_line_bytes = parseInt(netConsoleMaxAuthLineBytes, 10);
			if (!netconsole.max_record_bytes.env_override)
				patch.netconsole.max_record_bytes = parseInt(netConsoleMaxRecordBytes, 10);
			if (Object.keys(patch.netconsole).length === 0) delete patch.netconsole;
		}

		{
			const s = config.send;
			if (!s.dedup_ttl.env_override) {
				patch.send = { dedup_ttl: dedupTtl };
			}
		}

		{
			const rl = config.receive_log;
			if (
				!rl.enabled.env_override ||
				!rl.path.env_override ||
				!rl.retention_days.env_override ||
				!rl.replay_window.env_override
			) {
				patch.receive_log = {};
				if (!rl.enabled.env_override) patch.receive_log.enabled = receiveLogEnabled;
				if (!rl.path.env_override) patch.receive_log.path = receiveLogPath;
				if (!rl.retention_days.env_override) {
					const days = parseInt(receiveLogRetentionDays, 10);
					if (!isNaN(days) && days > 0) patch.receive_log.retention_days = days;
				}
				if (!rl.replay_window.env_override)
					patch.receive_log.replay_window = receiveLogReplayWindow;
			}
		}

		if (!config.request_log.enabled.env_override) {
			patch.request_log = { enabled: requestLogEnabled };
		}

		{
			const s = config.stats;
			if (!s.enabled.env_override || !s.path.env_override || !s.retention_days.env_override) {
				patch.stats = {};
				if (!s.enabled.env_override) patch.stats.enabled = statsEnabled;
				if (!s.path.env_override) patch.stats.path = statsPath;
				if (!s.retention_days.env_override) {
					const days = parseInt(statsRetentionDays, 10);
					if (!isNaN(days) && days > 0) patch.stats.retention_days = days;
				}
			}
		}

		{
			const cl = config.chat_log;
			if (
				!cl.path.env_override ||
				!cl.history_window.env_override ||
				!cl.max_history_window.env_override
			) {
				patch.chat_log = {};
				if (!cl.path.env_override) patch.chat_log.path = chatLogPath;
				if (!cl.history_window.env_override) patch.chat_log.history_window = chatLogHistoryWindow;
				if (!cl.max_history_window.env_override)
					patch.chat_log.max_history_window = chatLogMaxHistoryWindow;
			}
		}

		try {
			const result = await updateConfig(patch);
			config = result.config;
			populate(config);
			requiresRestart = result.requires_restart;
			saveSuccess = true;
			if (patch.my_call && connectionState.stationCallsign !== result.config.my_call.value) {
				connectionState.setStation({ callsign: result.config.my_call.value });
			}
		} catch (err) {
			saveError = err instanceof Error ? err.message : 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function shutdown() {
		shuttingDown = true;
		shutdownError = null;
		try {
			await shutdownApp();
		} catch (err) {
			shutdownError = err instanceof Error ? err.message : 'Shutdown failed';
			shuttingDown = false;
		}
		// On success: process is gone, no further state update needed.
	}

	async function restart() {
		restarting = true;
		restartError = null;
		try {
			const previousStartedAt = await getStartedAt();
			await restartApp();
			await waitForRestart(previousStartedAt);
			window.location.reload();
		} catch (err) {
			restartError = err instanceof Error ? err.message : 'Restart failed';
		} finally {
			restarting = false;
		}
	}
</script>

<div class="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
	<div class="mx-auto max-w-2xl space-y-5">
		<!-- Header -->
		<div class="rounded-2xl border border-warm/20 bg-surface/70 px-4 py-3.5 backdrop-blur-sm">
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-2.5">
					<span class="text-warm"><MdiIcon path={mdiTune} size={16} /></span>
					<h1 class="font-mono text-sm font-bold text-azure">Settings goMeshCom</h1>
				</div>
				{#if !loading && !loadError}
					<div class="mt-3 flex gap-1 border-t border-ink-dim/10 pt-3">
						{#each ['server', 'web', 'storage', 'system'] as const as key (key)}
							<button
								onclick={() => (activeSection = key)}
								class="rounded-lg px-3 py-1 text-xs font-medium transition-colors {activeSection ===
								key
									? 'bg-azure/15 text-azure'
									: 'text-ink-muted hover:bg-ink-dim/10 hover:text-ink'}"
							>
								{key === 'server'
									? 'Server'
									: key === 'web'
										? 'Web Interface'
										: key === 'storage'
											? 'Storage'
											: 'System'}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		</div>

		{#if loading}
			<p class="px-1 text-sm text-ink-muted">Loading configuration…</p>
		{:else if demoMode}
			<div class="rounded-2xl border border-warm/30 bg-warm/10 p-4 text-sm text-warm">
				<span class="font-semibold">Demo mode active.</span> Configuration is locked. To change
				settings, disable <code class="font-mono">demo_mode</code> in the TOML config file and restart.
			</div>
		{:else if loadError}
			<div class="rounded-2xl border border-coral/30 bg-coral/10 p-4 text-sm text-coral">
				{loadError}
				<button onclick={reload} class="ml-2 underline">Retry</button>
			</div>
		{:else if config}
			{#if restartError}
				<div class="rounded-2xl border border-coral/30 bg-coral/10 p-3 text-sm text-coral">
					{restartError}
				</div>
			{/if}
			{#if shutdownError}
				<div class="rounded-2xl border border-coral/30 bg-coral/10 p-3 text-sm text-coral">
					{shutdownError}
				</div>
			{/if}

			{#if activeSection === 'server'}
				<!-- Core -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiTune} size={14} /></span>
					<span>Core</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsField
							label="My callsign"
							description="Your station callsign. Changes apply live — no restart needed."
							envOverride={config.my_call.env_override}
							requiresRestart={config.my_call.requires_restart}
						>
							<input
								type="text"
								bind:value={myCall}
								disabled={config.my_call.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								placeholder="e.g. IU5PMP-1"
							/>
						</SettingsField>
						<SettingsField
							label="Log level"
							description="Verbosity of the application log."
							envOverride={config.log_level.env_override}
							requiresRestart={config.log_level.requires_restart}
						>
							<select
								bind:value={logLevel}
								disabled={config.log_level.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
							>
								{#each ['debug', 'info', 'warn', 'error'] as level (level)}
									<option value={level}>{level}</option>
								{/each}
							</select>
						</SettingsField>
						<SettingsField
							label="Transport"
							description="Node connection type. UDP remains the default for backward compatibility."
							envOverride={config.transport_mode.env_override}
							requiresRestart={config.transport_mode.requires_restart}
						>
							<select
								bind:value={transportMode}
								disabled={config.transport_mode.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
							>
								<option value="udp">UDP</option>
								<option value="serial">Serial</option>
								<option value="netconsole">NETConsole</option>
							</select>
						</SettingsField>
						{#if transportMode === 'udp'}
							<SettingsField
								label="UDP listen address"
								description="UDP address for incoming MeshCom packets."
								envOverride={config.udp_listen_addr.env_override}
								requiresRestart={config.udp_listen_addr.requires_restart}
							>
								<input
									type="text"
									bind:value={udpListenAddr}
									disabled={config.udp_listen_addr.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="0.0.0.0:1799"
								/>
							</SettingsField>
							<SettingsField
								label="Node address"
								description="MeshCom node UDP address. Leave empty to auto-detect from first packet."
								envOverride={config.node_addr.env_override}
								requiresRestart={config.node_addr.requires_restart}
							>
								<input
									type="text"
									bind:value={nodeAddr}
									disabled={config.node_addr.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="auto-detect"
								/>
							</SettingsField>
						{:else if transportMode === 'serial'}
							<SettingsField
								label="Serial device"
								description="Explicit device path or COM port. Automatic discovery is not used."
								envOverride={config.serial.device.env_override}
								requiresRestart={config.serial.device.requires_restart}
							>
								<input
									type="text"
									bind:value={serialDevice}
									disabled={config.serial.device.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="/dev/ttyUSB0 or COM3"
								/>
							</SettingsField>
							<SettingsField
								label="Baud rate"
								description="Serial speed. MeshCom firmware 4.35+ default: 115200."
								envOverride={config.serial.baud.env_override}
								requiresRestart={config.serial.baud.requires_restart}
							>
								<input
									type="number"
									bind:value={serialBaud}
									disabled={config.serial.baud.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									min="1"
								/>
							</SettingsField>
							<SettingsField
								label="Data bits"
								description="Serial data bits. Default: 8."
								envOverride={config.serial.data_bits.env_override}
								requiresRestart={config.serial.data_bits.requires_restart}
							>
								<select
									bind:value={serialDataBits}
									disabled={config.serial.data_bits.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								>
									{#each ['5', '6', '7', '8'] as bits (bits)}
										<option value={bits}>{bits}</option>
									{/each}
								</select>
							</SettingsField>
							<SettingsField
								label="Parity"
								description="Serial parity. Default: none."
								envOverride={config.serial.parity.env_override}
								requiresRestart={config.serial.parity.requires_restart}
							>
								<select
									bind:value={serialParity}
									disabled={config.serial.parity.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								>
									{#each ['none', 'odd', 'even', 'mark', 'space'] as parity (parity)}
										<option value={parity}>{parity}</option>
									{/each}
								</select>
							</SettingsField>
							<SettingsField
								label="Stop bits"
								description="Serial stop bits. Default: 1."
								envOverride={config.serial.stop_bits.env_override}
								requiresRestart={config.serial.stop_bits.requires_restart}
							>
								<select
									bind:value={serialStopBits}
									disabled={config.serial.stop_bits.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								>
									<option value="1">1</option>
									<option value="2">2</option>
								</select>
							</SettingsField>
							<SettingsField
								label="Flow control"
								description="Serial flow control. Default: none."
								envOverride={config.serial.flow_control.env_override}
								requiresRestart={config.serial.flow_control.requires_restart}
							>
								<select
									bind:value={serialFlowControl}
									disabled={config.serial.flow_control.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								>
									<option value="none">none</option>
								</select>
							</SettingsField>
							<SettingsField
								label="DTR"
								description="Disable for ESP32/CP2102 to avoid reset; enable for nRF52/RAK USB CDC."
								envOverride={config.serial.dtr.env_override}
								requiresRestart={config.serial.dtr.requires_restart}
							>
								<input
									type="checkbox"
									bind:checked={serialDtr}
									disabled={config.serial.dtr.env_override}
									class="h-4 w-4 accent-azure disabled:cursor-not-allowed disabled:opacity-50"
								/>
							</SettingsField>
							<SettingsField
								label="RTS"
								description="Keep disabled for ESP32/CP2102 unless hardware wiring requires it."
								envOverride={config.serial.rts.env_override}
								requiresRestart={config.serial.rts.requires_restart}
							>
								<input
									type="checkbox"
									bind:checked={serialRts}
									disabled={config.serial.rts.env_override}
									class="h-4 w-4 accent-azure disabled:cursor-not-allowed disabled:opacity-50"
								/>
							</SettingsField>
							<SettingsField
								label="Read timeout"
								description="Finite serial read timeout used for cancellation and reconnect handling."
								envOverride={config.serial.read_timeout.env_override}
								requiresRestart={config.serial.read_timeout.requires_restart}
							>
								<input
									type="text"
									bind:value={serialReadTimeout}
									disabled={config.serial.read_timeout.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="1s"
								/>
							</SettingsField>
							<SettingsField
								label="Reconnect initial"
								description="Initial delay after a failed connection."
								envOverride={config.serial.reconnect_initial.env_override}
								requiresRestart={config.serial.reconnect_initial.requires_restart}
							>
								<input
									type="text"
									bind:value={serialReconnectInitial}
									disabled={config.serial.reconnect_initial.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="1s"
								/>
							</SettingsField>
							<SettingsField
								label="Reconnect maximum"
								description="Maximum reconnect backoff."
								envOverride={config.serial.reconnect_max.env_override}
								requiresRestart={config.serial.reconnect_max.requires_restart}
							>
								<input
									type="text"
									bind:value={serialReconnectMax}
									disabled={config.serial.reconnect_max.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="30s"
								/>
							</SettingsField>
							<SettingsField
								label="Stable reset after"
								description="Connected duration required before resetting reconnect backoff."
								envOverride={config.serial.stable_reset_after.env_override}
								requiresRestart={config.serial.stable_reset_after.requires_restart}
							>
								<input
									type="text"
									bind:value={serialStableResetAfter}
									disabled={config.serial.stable_reset_after.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="30s"
								/>
							</SettingsField>
							<SettingsField
								label="Maximum serial record"
								description="Maximum buffered serial line size in bytes."
								envOverride={config.serial.max_record_bytes.env_override}
								requiresRestart={config.serial.max_record_bytes.requires_restart}
							>
								<input
									type="number"
									bind:value={serialMaxRecordBytes}
									disabled={config.serial.max_record_bytes.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									min="256"
									max="1048576"
								/>
							</SettingsField>
						{:else}
							<SettingsField
								label="NETConsole address"
								description="Raw TCP endpoint including port. Firmware default: 2323."
								envOverride={config.netconsole.address.env_override}
								requiresRestart={config.netconsole.address.requires_restart}
							>
								<input
									type="text"
									bind:value={netConsoleAddress}
									disabled={config.netconsole.address.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="192.168.1.53:2323"
								/>
							</SettingsField>
							<SettingsField
								label="NETConsole password"
								description="Stored locally and masked here. Empty selects firmware open-access mode."
								envOverride={config.netconsole.password.env_override}
								requiresRestart={config.netconsole.password.requires_restart}
							>
								<input
									type="password"
									bind:value={netConsolePassword}
									disabled={config.netconsole.password.env_override}
									autocomplete="new-password"
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="Open access"
								/>
							</SettingsField>
							<SettingsField
								label="Connect timeout"
								description="Maximum TCP connection attempt duration."
								envOverride={config.netconsole.connect_timeout.env_override}
								requiresRestart={config.netconsole.connect_timeout.requires_restart}
							>
								<input
									type="text"
									bind:value={netConsoleConnectTimeout}
									disabled={config.netconsole.connect_timeout.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="5s"
								/>
							</SettingsField>
							<SettingsField
								label="Authentication timeout"
								description="Maximum duration for challenge-response authentication."
								envOverride={config.netconsole.auth_timeout.env_override}
								requiresRestart={config.netconsole.auth_timeout.requires_restart}
							>
								<input
									type="text"
									bind:value={netConsoleAuthTimeout}
									disabled={config.netconsole.auth_timeout.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="5s"
								/>
							</SettingsField>
							<SettingsField
								label="Write timeout"
								description="Maximum duration for each socket write."
								envOverride={config.netconsole.write_timeout.env_override}
								requiresRestart={config.netconsole.write_timeout.requires_restart}
							>
								<input
									type="text"
									bind:value={netConsoleWriteTimeout}
									disabled={config.netconsole.write_timeout.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="5s"
								/>
							</SettingsField>
							<SettingsField
								label="Reconnect initial"
								description="Initial delay after a failed connection."
								envOverride={config.netconsole.reconnect_initial.env_override}
								requiresRestart={config.netconsole.reconnect_initial.requires_restart}
							>
								<input
									type="text"
									bind:value={netConsoleReconnectInitial}
									disabled={config.netconsole.reconnect_initial.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="1s"
								/>
							</SettingsField>
							<SettingsField
								label="Reconnect maximum"
								description="Maximum reconnect backoff."
								envOverride={config.netconsole.reconnect_max.env_override}
								requiresRestart={config.netconsole.reconnect_max.requires_restart}
							>
								<input
									type="text"
									bind:value={netConsoleReconnectMax}
									disabled={config.netconsole.reconnect_max.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="30s"
								/>
							</SettingsField>
							<SettingsField
								label="Stable reset after"
								description="Connected duration required before resetting reconnect backoff."
								envOverride={config.netconsole.stable_reset_after.env_override}
								requiresRestart={config.netconsole.stable_reset_after.requires_restart}
							>
								<input
									type="text"
									bind:value={netConsoleStableResetAfter}
									disabled={config.netconsole.stable_reset_after.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									placeholder="30s"
								/>
							</SettingsField>
							<SettingsField
								label="Maximum authentication line"
								description="Defensive authentication line limit in bytes."
								envOverride={config.netconsole.max_auth_line_bytes.env_override}
								requiresRestart={config.netconsole.max_auth_line_bytes.requires_restart}
							>
								<input
									type="number"
									bind:value={netConsoleMaxAuthLineBytes}
									disabled={config.netconsole.max_auth_line_bytes.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									min="72"
									max="4096"
								/>
							</SettingsField>
							<SettingsField
								label="Maximum NETConsole record"
								description="Maximum buffered console line size in bytes."
								envOverride={config.netconsole.max_record_bytes.env_override}
								requiresRestart={config.netconsole.max_record_bytes.requires_restart}
							>
								<input
									type="number"
									bind:value={netConsoleMaxRecordBytes}
									disabled={config.netconsole.max_record_bytes.env_override}
									class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
									min="1"
									max="1048576"
								/>
							</SettingsField>
							<div class="px-4 py-3 text-xs text-coral">
								NETConsole traffic is plaintext. Use only on a trusted LAN or through a VPN.
							</div>
						{/if}
						<SettingsField
							label="Max message length"
							description="Maximum outgoing message size in bytes (1–255)."
							envOverride={config.max_message_length.env_override}
							requiresRestart={config.max_message_length.requires_restart}
						>
							<input
								type="number"
								bind:value={maxMsgLen}
								disabled={config.max_message_length.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								min="1"
								max="255"
							/>
						</SettingsField>
					</div>
				</div>

				<!-- Send -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiSend} size={14} /></span>
					<span>Send</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsField
							label="Dedup TTL"
							description="Suppresses duplicate outgoing messages within this window. Set to 0s to disable."
							envOverride={config.send.dedup_ttl.env_override}
							requiresRestart={config.send.dedup_ttl.requires_restart}
						>
							<input
								type="text"
								bind:value={dedupTtl}
								disabled={config.send.dedup_ttl.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								placeholder="2s"
							/>
						</SettingsField>
					</div>
				</div>
			{/if}

			{#if activeSection === 'web'}
				<!-- HTTP -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiTune} size={14} /></span>
					<span>HTTP</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsField
							label="HTTP address"
							description="TCP address the HTTP server binds to."
							envOverride={config.http_addr.env_override}
							requiresRestart={config.http_addr.requires_restart}
						>
							<input
								type="text"
								bind:value={httpAddr}
								disabled={config.http_addr.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								placeholder="127.0.0.1:8080"
							/>
						</SettingsField>
					</div>
				</div>

				<!-- Request Log -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiSwapVertical} size={14} /></span>
					<span>Request Log</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsField
							label="Enabled"
							description="Log every HTTP request with method, path, status and duration."
							envOverride={config.request_log.enabled.env_override}
							requiresRestart={config.request_log.enabled.requires_restart}
						>
							<input
								type="checkbox"
								bind:checked={requestLogEnabled}
								disabled={config.request_log.enabled.env_override}
								class="h-4 w-4 cursor-pointer rounded accent-azure disabled:cursor-not-allowed disabled:opacity-50"
							/>
						</SettingsField>
					</div>
				</div>
			{/if}

			{#if activeSection === 'storage'}
				<!-- Receive Log -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiDownload} size={14} /></span>
					<span>Receive Log</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsField
							label="Enabled"
							description="Write received UDP packets to a daily JSONL log file."
							envOverride={config.receive_log.enabled.env_override}
							requiresRestart={config.receive_log.enabled.requires_restart}
						>
							<input
								type="checkbox"
								bind:checked={receiveLogEnabled}
								disabled={config.receive_log.enabled.env_override}
								class="h-4 w-4 cursor-pointer rounded accent-azure disabled:cursor-not-allowed disabled:opacity-50"
							/>
						</SettingsField>
						<SettingsField
							label="Path"
							description="Directory for daily received UDP JSONL files."
							envOverride={config.receive_log.path.env_override}
							requiresRestart={config.receive_log.path.requires_restart}
						>
							<input
								type="text"
								bind:value={receiveLogPath}
								disabled={config.receive_log.path.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
							/>
						</SettingsField>
						<SettingsField
							label="Retention"
							description="How many days of log files to keep."
							envOverride={config.receive_log.retention_days.env_override}
							requiresRestart={config.receive_log.retention_days.requires_restart}
						>
							<input
								type="number"
								bind:value={receiveLogRetentionDays}
								disabled={config.receive_log.retention_days.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								min="1"
							/>
						</SettingsField>
						<SettingsField
							label="Replay window"
							description="Recent packets replayed to new SSE subscribers on connect."
							envOverride={config.receive_log.replay_window.env_override}
							requiresRestart={config.receive_log.replay_window.requires_restart}
						>
							<input
								type="text"
								bind:value={receiveLogReplayWindow}
								disabled={config.receive_log.replay_window.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								placeholder="5m"
							/>
						</SettingsField>
					</div>
				</div>

				<!-- Stats -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiChartBar} size={14} /></span>
					<span>Stats</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsField
							label="Enabled"
							description="Collect hourly packet statistics."
							envOverride={config.stats.enabled.env_override}
							requiresRestart={config.stats.enabled.requires_restart}
						>
							<input
								type="checkbox"
								bind:checked={statsEnabled}
								disabled={config.stats.enabled.env_override}
								class="h-4 w-4 cursor-pointer rounded accent-azure disabled:cursor-not-allowed disabled:opacity-50"
							/>
						</SettingsField>
						<SettingsField
							label="Path"
							description="File where statistics are persisted."
							envOverride={config.stats.path.env_override}
							requiresRestart={config.stats.path.requires_restart}
						>
							<input
								type="text"
								bind:value={statsPath}
								disabled={config.stats.path.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
							/>
						</SettingsField>
						<SettingsField
							label="Retention"
							description="How many days of hourly buckets to keep."
							envOverride={config.stats.retention_days.env_override}
							requiresRestart={config.stats.retention_days.requires_restart}
						>
							<input
								type="number"
								bind:value={statsRetentionDays}
								disabled={config.stats.retention_days.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								min="1"
							/>
						</SettingsField>
					</div>
				</div>

				<!-- Chat Log -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiMessageTextOutline} size={14} /></span>
					<span>Chat Log</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsField
							label="Path"
							description="Directory for chat JSONL files."
							envOverride={config.chat_log.path.env_override}
							requiresRestart={config.chat_log.path.requires_restart}
						>
							<input
								type="text"
								bind:value={chatLogPath}
								disabled={config.chat_log.path.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
							/>
						</SettingsField>
						<SettingsField
							label="History window"
							description="Default time window returned by chat history queries. Changes apply live."
							envOverride={config.chat_log.history_window.env_override}
							requiresRestart={config.chat_log.history_window.requires_restart}
						>
							<input
								type="text"
								bind:value={chatLogHistoryWindow}
								disabled={config.chat_log.history_window.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								placeholder="24h"
							/>
						</SettingsField>
						<SettingsField
							label="Max history window"
							description="Upper limit for the ?hours= query parameter. Changes apply live."
							envOverride={config.chat_log.max_history_window.env_override}
							requiresRestart={config.chat_log.max_history_window.requires_restart}
						>
							<input
								type="text"
								bind:value={chatLogMaxHistoryWindow}
								disabled={config.chat_log.max_history_window.env_override}
								class="w-full rounded-lg border border-ink-dim/30 bg-base px-2.5 py-1.5 font-mono text-xs text-ink focus:border-azure focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"
								placeholder="168h"
							/>
						</SettingsField>
					</div>
				</div>
			{/if}

			{#if activeSection === 'server'}
				<!-- Forward -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiArrowRight} size={14} /></span>
					<span>Forward</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsInfoRow
							label="Targets"
							description="Comma-separated host:port list — incoming UDP packets are mirrored to each target."
							value={config.forward.targets.value || '(none)'}
							envOverride={config.forward.targets.env_override}
						/>
					</div>
				</div>
			{/if}

			{#if activeSection === 'system'}
				<!-- About -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiInformationOutline} size={14} /></span>
					<span>About</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<SettingsInfoRow label="Version" value={config.server.version || '—'} mono />
						<SettingsInfoRow label="Started at" value={formatStartedAt(config.server.started_at)} />
						<SettingsInfoRow
							label="Uptime"
							value={formatUptime(config.server.uptime_seconds)}
							mono
						/>
					</div>
				</div>

				<!-- Restart -->
				<div class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-ink-muted">
					<span class="text-ink-muted"><MdiIcon path={mdiTune} size={14} /></span>
					<span>Process</span>
					<span class="h-px flex-1 bg-ink-dim/20"></span>
				</div>
				<div class="-mt-2 rounded-2xl border border-ink-dim/15 bg-surface/50 backdrop-blur-sm">
					<div class="divide-y divide-ink-dim/15">
						<div class="flex items-center justify-between px-4 py-3">
							<div>
								<p class="text-sm font-medium text-ink">Restart</p>
								<p class="mt-0.5 text-xs text-ink-muted">
									Gracefully restart the daemon. Existing connections are closed and the process
									re-executes.
								</p>
							</div>
							<button
								onclick={restart}
								disabled={restarting || shuttingDown}
								class="ml-6 shrink-0 rounded-lg border border-azure/40 px-4 py-1.5 text-sm font-medium text-azure transition-colors hover:bg-azure/10 disabled:cursor-not-allowed disabled:opacity-50"
							>
								{restarting ? 'Restarting…' : 'Restart'}
							</button>
						</div>
						{#if restartError}
							<div class="px-4 py-2 text-xs text-coral">{restartError}</div>
						{/if}
						<div class="flex items-center justify-between px-4 py-3">
							<div>
								<p class="text-sm font-medium text-ink">Shutdown</p>
								<p class="mt-0.5 text-xs text-ink-muted">
									Stop the daemon. The process exits cleanly without restarting.
								</p>
							</div>
							<button
								onclick={shutdown}
								disabled={restarting || shuttingDown}
								class="ml-6 shrink-0 rounded-lg border border-coral/40 px-4 py-1.5 text-sm font-medium text-coral transition-colors hover:bg-coral/10 disabled:cursor-not-allowed disabled:opacity-50"
							>
								{shuttingDown ? 'Shutting down…' : 'Shutdown'}
							</button>
						</div>
						{#if shutdownError}
							<div class="px-4 py-2 text-xs text-coral">{shutdownError}</div>
						{/if}
					</div>
				</div>
			{/if}

			<!-- Bottom Save bar -->
			{#if activeSection !== 'system'}
				<div
					class="flex items-center justify-end gap-3 rounded-2xl border border-ink-dim/15 bg-surface/50 px-4 py-3 backdrop-blur-sm"
				>
					{#if saveSuccess && !isDirty}
						<span class="text-xs text-mint">✓ Saved.</span>
					{/if}
					{#if saveError}
						<span class="text-xs text-coral">{saveError}</span>
					{/if}
					{#if requiresRestart && !isDirty}
						<button
							onclick={restart}
							disabled={restarting || shuttingDown}
							class="rounded-lg border border-amber-500/40 px-4 py-1.5 text-sm font-medium text-amber-400 transition-colors hover:bg-amber-500/10 disabled:cursor-not-allowed disabled:opacity-50"
						>
							{restarting ? 'Restarting…' : 'Restart to apply'}
						</button>
					{/if}
					<button
						onclick={save}
						disabled={!isDirty || saving || restarting || shuttingDown}
						class="rounded-lg bg-azure px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-azure/80 disabled:cursor-not-allowed disabled:opacity-40"
					>
						{saving ? 'Saving…' : 'Save'}
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>
