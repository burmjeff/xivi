<script lang="ts">
	import Epg from '@xivi/components/Epg.svelte';
	import Icon from '@iconify/svelte';
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-gradient font-bold mb-1">EPG Management</h1>
				<p class="text-surface-300">Manage your Electronic Program Guide sources</p>
			</div>

			<div>
				<button class="btn bg-primary-700 hover:bg-primary-600 text-white px-4 py-2 rounded-lg transition-colors duration-200 flex items-center gap-2 self-start">
					<Icon icon="mdi:refresh" width="18" height="18" />
					<span>Refresh All EPGs</span>
				</button>
			</div>
		</div>
	</header>

	<!-- Info Card -->
	<div class="card bg-tertiary-900/10 border border-tertiary-500/30 p-4 rounded-lg mb-4 flex items-center gap-3">
		<Icon icon="mdi:information-outline" class="text-tertiary-400" width="24" height="24" />
		<p class="text-surface-200">
			EPG (Electronic Program Guide) provides program scheduling information for your channels.
			Add EPG sources below to enhance your viewing experience with program information.
		</p>
	</div>

	<!-- EPG Component -->
	<div class="animate-slide-in">
		<Epg />
	</div>
</div>
