<script lang="ts">
	import { onMount } from 'svelte';
	import { API_BASE } from '$lib/api/events';

	type TransportStatus = {
		mode: 'udp' | 'serial' | 'netconsole';
		state: string;
		last_error?: string;
	};

	type HealthResponse = {
		transport?: TransportStatus;
	};

	const { transport: initialTransport }: { transport?: TransportStatus } = $props();
	let transport = $state<TransportStatus | undefined>();

	async function refresh() {
		try {
			const response = await fetch(`${API_BASE}/health`, { cache: 'no-store' });
			if (!response.ok) return;
			transport = ((await response.json()) as HealthResponse).transport;
		} catch {
			// Connection indicator reports an unavailable API connection.
		}
	}

	onMount(() => {
		if (initialTransport) {
			transport = initialTransport;
			return;
		}
		void refresh();
		const interval = window.setInterval(() => void refresh(), 5000);
		return () => window.clearInterval(interval);
	});

	let connectionTransportUnavailable = $derived(
		(transport?.mode === 'serial' || transport?.mode === 'netconsole') &&
			transport.state !== 'connected'
	);
	let unavailableLabel = $derived(
		transport?.mode === 'netconsole' ? 'NETConsole unavailable' : 'Serial unavailable'
	);
</script>

{#if connectionTransportUnavailable}
	<span
		data-testid="transport-warning"
		class="max-w-28 shrink-0 truncate whitespace-nowrap rounded-full bg-coral/15 px-2.5 py-0.5 text-[11px] font-semibold text-coral md:max-w-none"
		title={transport?.last_error}
	>
		{unavailableLabel}{transport?.last_error ? `: ${transport.last_error}` : ''}
	</span>
{/if}
