<script lang="ts">
	import Epg from '@xivi/components/Epg.svelte';
	import Icon from '@iconify/svelte';
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-gradient mb-1 font-bold">EPG Management</h1>
				<p class="text-surface-300">Manage your Electronic Program Guide sources</p>
			</div>

			<div>
				<button
					class="btn bg-primary-700 hover:bg-primary-600 flex items-center gap-2 self-start rounded-lg px-4 py-2 text-white transition-colors duration-200"
				>
					<Icon icon="mdi:refresh" width="18" height="18" />
					<span>Refresh All EPGs</span>
				</button>
			</div>
		</div>
	</header>

	<!-- Info Card -->
	<div
		class="card bg-tertiary-900/10 border-tertiary-500/30 mb-4 flex items-center gap-3 rounded-lg border p-4"
	>
		<Icon icon="mdi:information-outline" class="text-tertiary-400" width="24" height="24" />
		<p class="text-surface-200">
			EPG (Electronic Program Guide) provides program scheduling information for your channels. Add
			EPG sources below to enhance your viewing experience with program information.
		</p>
	</div>

	<!-- EPG Component -->
	<div class="animate-slide-in">
		<Epg />
	</div>
</div>
