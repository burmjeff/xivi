<script>
	import Settings from '@xivi/components/Settings.svelte';
	import Icon from '@iconify/svelte';
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-2">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-gradient mb-2 font-bold">System Settings</h1>
				<p class="text-surface-300">Configure your Xivi server settings</p>
			</div>

			<div>
				<button
					class="btn bg-success-700 hover:bg-success-600 flex items-center gap-2 rounded-lg px-4 py-2 text-white transition-colors duration-200"
				>
					<Icon icon="mdi:content-save" width="18" height="18" />
					<span>Save All Settings</span>
				</button>
			</div>
		</div>
	</header>

	<!-- Warning Card -->
	<div
		class="card bg-warning-900/10 border-warning-500/30 mb-6 flex items-center gap-3 rounded-lg border p-4"
	>
		<Icon icon="mdi:alert-circle-outline" class="text-warning-400" width="24" height="24" />
		<p class="text-surface-200">
			Changing some settings may require a server restart to take effect. Make sure to save your
			changes before restarting the server.
		</p>
	</div>

	<!-- Settings Component -->
	<div
		class="card bg-surface-800/30 border-surface-700/50 animate-slide-in rounded-lg border p-6 shadow-lg"
	>
		<Settings />
	</div>
</div>
