<script>
	import Viewer from '@xivi/components/Viewer.svelte';
	import Icon from '@iconify/svelte';
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-2">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-gradient mb-1 font-bold text-4xl">Stream Viewer</h1>
				<p class="text-surface-300">Browse and watch your channels</p>
			</div>

			<div class="flex items-center gap-3">
				<button
					class="btn bg-surface-700/50 hover:bg-surface-700 flex items-center gap-2 rounded-lg px-4 py-2 text-white transition-colors duration-200"
				>
					<Icon icon="mdi:view-grid" width="18" height="18" />
					<span>Grid View</span>
				</button>

				<button
					class="btn bg-surface-800/50 hover:bg-surface-700 flex items-center gap-2 rounded-lg px-4 py-2 text-white transition-colors duration-200"
				>
					<Icon icon="mdi:view-list" width="18" height="18" />
					<span>List View</span>
				</button>
			</div>
		</div>
	</header>

	<!-- Viewer Component -->
	<div class="animate-slide-in">
		<Viewer />
	</div>
</div>
