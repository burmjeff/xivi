<script lang="ts">
	import TemplateStatus from '@xivi/components/TemplateStatus.svelte';
	import Icon from '@iconify/svelte';
	import { onMount, onDestroy } from 'svelte';
	import type { SystemStatusState, SystemStatusResponse } from '@xivi/data/system_entities';

	// System status data
	let systemStatus: SystemStatusState = $state({
		uptime: '0 days, 0 hours, 0 minutes',
		cpu: '0%',
		memory: '0 MB / 0 MB (0%)',
		connections: 0,
		isLoading: true,
		error: null,
		lastUpdated: null
	});

	// Auto-refresh interval
	let refreshInterval: ReturnType<typeof setInterval> | null = null;
	const REFRESH_INTERVAL_MS = 30000; // 30 seconds

	// Fetch system status data from API
	async function fetchSystemStatus() {
		try {
			systemStatus.isLoading = true;
			systemStatus.error = null;

			const response = await fetch('/api/system/status');

			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`);
			}

			const data: SystemStatusResponse = await response.json();

			if (data.error) {
				throw new Error(data.msg || 'Unknown API error');
			}

			// Update system status with API data
			systemStatus = {
				uptime: data.status.uptime,
				cpu: data.status.cpu,
				memory: data.status.memory,
				connections: data.status.connections,
				isLoading: false,
				error: null,
				lastUpdated: Date.now()
			};
		} catch (error) {
			console.error('Error fetching system status:', error);
			systemStatus = {
				...systemStatus,
				isLoading: false,
				error: error instanceof Error ? error.message : 'Failed to fetch system status',
				lastUpdated: Date.now()
			};
		}
	}

	// Start auto-refresh
	function startAutoRefresh() {
		if (refreshInterval) {
			clearInterval(refreshInterval);
		}
		refreshInterval = setInterval(fetchSystemStatus, REFRESH_INTERVAL_MS);
	}

	// Stop auto-refresh
	function stopAutoRefresh() {
		if (refreshInterval) {
			clearInterval(refreshInterval);
			refreshInterval = null;
		}
	}

	// Manual refresh handler
	async function handleRefresh() {
		await fetchSystemStatus();
		// Restart auto-refresh timer
		startAutoRefresh();
	}

	onMount(() => {
		fetchSystemStatus();
		startAutoRefresh();
	});

	onDestroy(() => {
		stopAutoRefresh();
	});
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-6">
		<h1 class="text-gradient mb-1 font-bold text-4xl">System Dashboard</h1>
		<div class="flex items-end justify-between">
			<div>
				<p class="text-surface-300">Monitor your Xivi server status and templates</p>
				{#if systemStatus.lastUpdated && !systemStatus.error}
					<p class="text-surface-400 mt-1 text-sm">
						Last updated: {new Date(systemStatus.lastUpdated).toLocaleTimeString()}
						• Auto-refresh every {REFRESH_INTERVAL_MS / 1000}s
					</p>
				{/if}
			</div>
			<!-- Refresh Button -->
			<div class="flex justify-end">
				<button
					class="btn bg-primary-700 hover:bg-primary-600 flex items-center gap-2 rounded-lg px-4 py-2 text-white transition-colors duration-200 disabled:cursor-not-allowed disabled:opacity-50"
					onclick={handleRefresh}
					disabled={systemStatus.isLoading}
				>
					<Icon
						icon="mdi:refresh"
						width="18"
						height="18"
						class={systemStatus.isLoading ? 'animate-spin' : ''}
					/>
					<span>{systemStatus.isLoading ? 'Refreshing...' : 'Refresh Status'}</span>
				</button>
			</div>
		</div>
	</header>

	<!-- Error Banner -->
	{#if systemStatus.error}
		<div class="alert variant-filled-error mb-6">
			<Icon icon="mdi:alert-circle" width="20" height="20" />
			<div class="alert-message">
				<h3 class="h4">System Status Error</h3>
				<p>{systemStatus.error}</p>
			</div>
			<div class="alert-actions">
				<button class="btn variant-filled" onclick={handleRefresh}>
					<Icon icon="mdi:refresh" width="16" height="16" />
					<span>Retry</span>
				</button>
			</div>
		</div>
	{/if}

	<!-- Status Cards Grid -->
	<div class="mb-8 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
		<!-- Uptime Card -->
		<div class="card bg-primary-900/10 border-primary-500/30 flex flex-col border p-4">
			<div class="mb-2 flex items-center gap-3">
				<Icon icon="mdi:clock-outline" class="text-primary-400" width="24" height="24" />
				<h3 class="text-lg font-medium">Uptime</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="bg-surface-700/50 my-2 h-6 w-3/4 animate-pulse rounded"></div>
			{:else if systemStatus.error}
				<p class="text-error-400 text-xl font-semibold">--</p>
			{:else}
				<p class="text-primary-300 text-xl font-semibold">{systemStatus.uptime}</p>
			{/if}
		</div>

		<!-- CPU Usage Card -->
		<div class="card bg-secondary-900/10 border-secondary-500/30 flex flex-col border p-4">
			<div class="mb-2 flex items-center gap-3">
				<Icon icon="mdi:cpu-64-bit" class="text-secondary-400" width="24" height="24" />
				<h3 class="text-lg font-medium">CPU Usage</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="bg-surface-700/50 my-2 h-6 w-1/4 animate-pulse rounded"></div>
			{:else if systemStatus.error}
				<p class="text-error-400 text-xl font-semibold">--</p>
			{:else}
				<p class="text-secondary-300 text-xl font-semibold">{systemStatus.cpu}</p>
			{/if}
		</div>

		<!-- Memory Usage Card -->
		<div class="card bg-tertiary-900/10 border-tertiary-500/30 flex flex-col border p-4">
			<div class="mb-2 flex items-center gap-3">
				<Icon icon="mdi:memory" class="text-tertiary-400" width="24" height="24" />
				<h3 class="text-lg font-medium">Memory Usage</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="bg-surface-700/50 my-2 h-6 w-2/5 animate-pulse rounded"></div>
			{:else if systemStatus.error}
				<p class="text-error-400 text-xl font-semibold">--</p>
			{:else}
				<p class="text-tertiary-300 text-xl font-semibold">{systemStatus.memory}</p>
			{/if}
		</div>

		<!-- Active Connections Card -->
		<div class="card bg-success-900/10 border-success-500/30 flex flex-col border p-4">
			<div class="mb-2 flex items-center gap-3">
				<Icon icon="mdi:connection" class="text-success-400" width="24" height="24" />
				<h3 class="text-lg font-medium">Connections</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="bg-surface-700/50 my-2 h-6 w-1/3 animate-pulse rounded"></div>
			{:else if systemStatus.error}
				<p class="text-error-400 text-xl font-semibold">--</p>
			{:else}
				<p class="text-success-300 text-xl font-semibold">{systemStatus.connections}</p>
			{/if}
		</div>
	</div>

	<!-- Template Status Section -->
	<div class="animate-slide-in">
		<TemplateStatus />
	</div>
</div>
