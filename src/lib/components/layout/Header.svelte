<script lang="ts">
	import xivi from '@xivi/lib/assets/xivi.png';
	import Icon from '@iconify/svelte';
</script>

<div class="glass-header flex h-16 items-center justify-between px-6">
	<div class="flex items-center gap-3">
		<div class="relative">
			<div class="bg-primary-500/20 absolute -inset-1 rounded-full blur-sm"></div>
			<img
				class="relative h-8 w-auto transition-transform duration-300 hover:scale-105"
				src={xivi}
				alt="Xivi Logo"
			/>
		</div>
		<span class="text-surface-50 text-xl font-bold tracking-tight">Xivi</span>
	</div>

	<div class="flex items-center gap-4">
		<a
			class="group border-surface-700 bg-surface-800/50 text-surface-300 hover:border-primary-500/50 hover:bg-surface-800 hover:text-primary-400 flex items-center gap-2 rounded-full border px-4 py-1.5 text-sm font-medium transition-all duration-200 hover:shadow-[0_0_15px_-3px_rgba(168,85,247,0.15)]"
			href="https://github.com/burmjeff/xivi"
			target="_blank"
			rel="noreferrer"
		>
			<Icon icon="mdi:github" width="18" height="18" />
			<span>GitHub</span>
		</a>
	</div>
</div>
