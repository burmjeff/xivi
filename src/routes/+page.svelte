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
	let refreshInterval: number | null = null;
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
		<h1 class="text-gradient font-bold mb-2">System Dashboard</h1>
		<div class="flex justify-between items-end">
			<div>
				<p class="text-surface-300">Monitor your Xivi server status and templates</p>
				{#if systemStatus.lastUpdated && !systemStatus.error}
					<p class="text-surface-400 text-sm mt-1">
						Last updated: {new Date(systemStatus.lastUpdated).toLocaleTimeString()}
						• Auto-refresh every {REFRESH_INTERVAL_MS / 1000}s
					</p>
				{/if}
			</div>
			<!-- Refresh Button -->
			<div class="flex justify-end">
				<button
					class="btn bg-primary-700 hover:bg-primary-600 text-white flex items-center gap-2 px-4 py-2 rounded-lg transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
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
	<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
		<!-- Uptime Card -->
		<div class="card p-4 bg-primary-900/10 border border-primary-500/30 flex flex-col">
			<div class="flex items-center gap-3 mb-2">
				<Icon icon="mdi:clock-outline" class="text-primary-400" width="24" height="24" />
				<h3 class="text-lg font-medium">Uptime</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="h-6 bg-surface-700/50 rounded animate-pulse w-3/4 my-2"></div>
			{:else if systemStatus.error}
				<p class="text-xl font-semibold text-error-400">--</p>
			{:else}
				<p class="text-xl font-semibold text-primary-300">{systemStatus.uptime}</p>
			{/if}
		</div>

		<!-- CPU Usage Card -->
		<div class="card p-4 bg-secondary-900/10 border border-secondary-500/30 flex flex-col">
			<div class="flex items-center gap-3 mb-2">
				<Icon icon="mdi:cpu-64-bit" class="text-secondary-400" width="24" height="24" />
				<h3 class="text-lg font-medium">CPU Usage</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="h-6 bg-surface-700/50 rounded animate-pulse w-1/4 my-2"></div>
			{:else if systemStatus.error}
				<p class="text-xl font-semibold text-error-400">--</p>
			{:else}
				<p class="text-xl font-semibold text-secondary-300">{systemStatus.cpu}</p>
			{/if}
		</div>

		<!-- Memory Usage Card -->
		<div class="card p-4 bg-tertiary-900/10 border border-tertiary-500/30 flex flex-col">
			<div class="flex items-center gap-3 mb-2">
				<Icon icon="mdi:memory" class="text-tertiary-400" width="24" height="24" />
				<h3 class="text-lg font-medium">Memory Usage</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="h-6 bg-surface-700/50 rounded animate-pulse w-2/5 my-2"></div>
			{:else if systemStatus.error}
				<p class="text-xl font-semibold text-error-400">--</p>
			{:else}
				<p class="text-xl font-semibold text-tertiary-300">{systemStatus.memory}</p>
			{/if}
		</div>

		<!-- Active Connections Card -->
		<div class="card p-4 bg-success-900/10 border border-success-500/30 flex flex-col">
			<div class="flex items-center gap-3 mb-2">
				<Icon icon="mdi:connection" class="text-success-400" width="24" height="24" />
				<h3 class="text-lg font-medium">Connections</h3>
			</div>
			{#if systemStatus.isLoading}
				<div class="h-6 bg-surface-700/50 rounded animate-pulse w-1/3 my-2"></div>
			{:else if systemStatus.error}
				<p class="text-xl font-semibold text-error-400">--</p>
			{:else}
				<p class="text-xl font-semibold text-success-300">{systemStatus.connections}</p>
			{/if}
		</div>
	</div>

	<!-- Template Status Section -->
	<div class="animate-slide-in">
		<TemplateStatus />
	</div>
</div>
