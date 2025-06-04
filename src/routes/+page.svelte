<script>
	import TemplateStatus from '@xivi/components/TemplateStatus.svelte';
	import Icon from '@iconify/svelte';
	import { onMount } from 'svelte';

	// System status data
	let systemStatus = $state({
		uptime: '0 days, 0 hours, 0 minutes',
		cpu: '0%',
		memory: '0 MB',
		connections: 0,
		isLoading: true
	});

	// Fetch system status data
	async function fetchSystemStatus() {
		// In a real implementation, this would fetch from an API
		// For now, we'll simulate with random data
		systemStatus.isLoading = true;

		// Simulate API call
		setTimeout(() => {
			const days = Math.floor(Math.random() * 30);
			const hours = Math.floor(Math.random() * 24);
			const minutes = Math.floor(Math.random() * 60);

			systemStatus = {
				uptime: `${days} days, ${hours} hours, ${minutes} minutes`,
				cpu: `${Math.floor(Math.random() * 50)}%`,
				memory: `${Math.floor(Math.random() * 1000)} MB`,
				connections: Math.floor(Math.random() * 100),
				isLoading: false
			};
		}, 1000);
	}

	onMount(() => {
		fetchSystemStatus();
	});
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-6">
		<h1 class="text-gradient font-bold mb-2">System Dashboard</h1>
		<div class="flex justify-between">
			<p class="text-surface-300">Monitor your Xivi server status and templates</p>
			<!-- Refresh Button -->
			<div class="flex justify-end">
				<button
					class="btn bg-primary-700 hover:bg-primary-600 text-white flex items-center gap-2 px-4 py-2 rounded-lg transition-colors duration-200"
					onclick={fetchSystemStatus}
				>
					<Icon icon="mdi:refresh" width="18" height="18" />
					<span>Refresh Status</span>
				</button>
			</div>
		</div>
	</header>

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
