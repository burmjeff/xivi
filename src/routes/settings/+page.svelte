<script>
	import Settings from '@xivi/components/Settings.svelte';
	import Icon from '@iconify/svelte';
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-2">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-gradient font-bold mb-2">System Settings</h1>
				<p class="text-surface-300">Configure your Xivi server settings</p>
			</div>

			<div>
				<button class="btn bg-success-700 hover:bg-success-600 text-white px-4 py-2 rounded-lg transition-colors duration-200 flex items-center gap-2">
					<Icon icon="mdi:content-save" width="18" height="18" />
					<span>Save All Settings</span>
				</button>
			</div>
		</div>
	</header>

	<!-- Warning Card -->
	<div class="card bg-warning-900/10 border border-warning-500/30 p-4 rounded-lg mb-6 flex items-center gap-3">
		<Icon icon="mdi:alert-circle-outline" class="text-warning-400" width="24" height="24" />
		<p class="text-surface-200">
			Changing some settings may require a server restart to take effect.
			Make sure to save your changes before restarting the server.
		</p>
	</div>

	<!-- Settings Component -->
	<div class="card bg-surface-800/30 border border-surface-700/50 shadow-lg rounded-lg p-6 animate-slide-in">
		<Settings />
	</div>
</div>
