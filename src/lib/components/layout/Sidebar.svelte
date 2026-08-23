<script lang="ts">
	import Icon from '@iconify/svelte';
	import { page } from '$app/stores';

	let currentPath = $derived($page.url.pathname);

	function isActive(path: string): boolean {
		return currentPath === path;
	}

	const menuItems = [
		{
			path: '/',
			icon: 'mdi:home-variant-outline',
			activeIcon: 'mdi:home-variant',
			label: 'Status'
		},
		{
			path: '/viewer',
			icon: 'mdi:television',
			activeIcon: 'mdi:television-classic',
			label: 'Viewer'
		},
		{
			path: '/channels',
			icon: 'mdi:playlist-play',
			activeIcon: 'mdi:playlist-play',
			label: 'Channels'
		},
		{
			path: '/epg',
			icon: 'mdi:calendar-clock-outline',
			activeIcon: 'mdi:calendar-clock',
			label: 'EPG'
		},
		{ path: '/settings', icon: 'mdi:cog-outline', activeIcon: 'mdi:cog', label: 'Settings' }
	];
</script>

<nav class="h-full px-3 py-6">
	<div class="text-surface-500 mb-6 px-3 text-xs font-semibold tracking-wider uppercase">Menu</div>

	<ul class="space-y-1">
		{#each menuItems as item}
			<li>
				<a
					href={item.path}
					class="group flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-200
					{isActive(item.path)
						? 'bg-primary-500/10 text-primary-400 ring-primary-500/20 shadow-sm ring-1'
						: 'text-surface-400 hover:bg-surface-800/50 hover:text-surface-200'}"
				>
					<div class="relative flex h-5 w-5 items-center justify-center">
						<Icon
							icon={isActive(item.path) ? item.activeIcon : item.icon}
							width="20"
							height="20"
							class="transition-transform duration-200 group-hover:scale-110"
						/>
					</div>
					<span>{item.label}</span>

					{#if isActive(item.path)}
						<div
							class="bg-primary-500 absolute left-0 h-4 w-0.5 rounded-r-full opacity-0 transition-opacity duration-200"
						></div>
					{/if}
				</a>
			</li>
		{/each}
	</ul>

	<div class="text-surface-500 mt-8 mb-4 px-3 text-xs font-semibold tracking-wider uppercase">
		System
	</div>

	<ul class="space-y-1">
		<li>
			<a
				href="https://github.com/burmjeff/xivi"
				target="_blank"
				rel="noreferrer"
				class="group text-surface-400 hover:bg-surface-800/50 hover:text-surface-200 flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all duration-200"
			>
				<Icon icon="mdi:github" width="20" height="20" />
				<span>GitHub</span>
			</a>
		</li>
	</ul>
</nav>
