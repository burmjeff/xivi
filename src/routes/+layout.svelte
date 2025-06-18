<script lang="ts">
	import '@xivi/app.css';
	import { AppBar } from '@skeletonlabs/skeleton-svelte';
	import xivi from '@xivi/lib/assets/xivi.png';
	import '../hmr-handler';
	import Icon from '@iconify/svelte';

	interface Props {
		children?: import('svelte').Snippet;
	}

	let { children }: Props = $props();

	// Active route tracking
	let currentPath = $state('/');

	// Update active route on client-side
	function updateCurrentPath() {
		if (typeof window !== 'undefined') {
			currentPath = window.location.pathname;
		}
	}

	// Check if a route is active
	function isActive(path: string): boolean {
		return currentPath === path;
	}

	// Update on mount and navigation
	$effect(() => {
		updateCurrentPath();

		if (typeof window !== 'undefined') {
			// Listen for route changes
			const handleRouteChange = () => updateCurrentPath();
			window.addEventListener('popstate', handleRouteChange);

			return () => {
				window.removeEventListener('popstate', handleRouteChange);
			};
		}
	});
</script>

<!-- Semantic layout with Tailwind grid -->
<div class="grid h-full grid-cols-[auto_1fr] grid-rows-[auto_1fr_auto]">
	<!-- Header (spans both columns) -->
	<header class="sticky top-0 z-10 col-span-2 shadow-md">
		<!-- App Bar -->
		<AppBar classes="h-16 justify-center bg-gradient-to-r from-surface-900 to-surface-800">
			{#snippet lead()}
				<div class="flex items-center gap-2">
					<img
						class="h-10 w-auto transition-transform duration-300 hover:scale-110"
						src={xivi}
						alt="Xivi Logo"
					/>
				</div>
			{/snippet}
			{#snippet trail()}
				<div class="flex items-center gap-4">
					<a
						class="btn btn-sm bg-primary-700 hover:bg-primary-600 flex items-center gap-2 text-white transition-colors duration-200"
						href="https://github.com/burmjeff/xivi"
						target="_blank"
						rel="noreferrer"
					>
						<Icon icon="mdi:github" width="18" height="18" />
						<span>GitHub</span>
					</a>
				</div>
			{/snippet}
		</AppBar>
	</header>

	<!-- Sidebar -->
	<aside class="bg-surface-800/90 w-60 shadow-lg">
		<!-- Navigation -->
		<nav class="px-4 py-6">
			<ul class="space-y-1">
				<li>
					<a
						href="/"
						class="flex items-center gap-3 rounded-lg px-4 py-3 transition-all duration-200 {isActive(
							'/'
						)
							? 'bg-primary-900/50 text-primary-400 font-medium'
							: 'hover:bg-surface-700/50'}"
						onclick={() => (currentPath = '/')}
					>
						<Icon icon="mdi:home" width="20" height="20" />
						<span>Status</span>
					</a>
				</li>
				<li>
					<a
						href="/viewer"
						class="flex items-center gap-3 rounded-lg px-4 py-3 transition-all duration-200 {isActive(
							'/viewer'
						)
							? 'bg-primary-900/50 text-primary-400 font-medium'
							: 'hover:bg-surface-700/50'}"
						onclick={() => (currentPath = '/viewer')}
					>
						<Icon icon="mdi:television" width="20" height="20" />
						<span>Viewer</span>
					</a>
				</li>
				<li>
					<a
						href="/channels"
						class="flex items-center gap-3 rounded-lg px-4 py-3 transition-all duration-200 {isActive(
							'/channels'
						)
							? 'bg-primary-900/50 text-primary-400 font-medium'
							: 'hover:bg-surface-700/50'}"
						onclick={() => (currentPath = '/channels')}
					>
						<Icon icon="mdi:playlist-play" width="20" height="20" />
						<span>Channel Management</span>
					</a>
				</li>
				<li>
					<a
						href="/epg"
						class="flex items-center gap-3 rounded-lg px-4 py-3 transition-all duration-200 {isActive(
							'/epg'
						)
							? 'bg-primary-900/50 text-primary-400 font-medium'
							: 'hover:bg-surface-700/50'}"
						onclick={() => (currentPath = '/epg')}
					>
						<Icon icon="mdi:calendar-clock" width="20" height="20" />
						<span>EPG</span>
					</a>
				</li>
				<li>
					<a
						href="/settings"
						class="flex items-center gap-3 rounded-lg px-4 py-3 transition-all duration-200 {isActive(
							'/settings'
						)
							? 'bg-primary-900/50 text-primary-400 font-medium'
							: 'hover:bg-surface-700/50'}"
						onclick={() => (currentPath = '/settings')}
					>
						<Icon icon="mdi:cog" width="20" height="20" />
						<span>Settings</span>
					</a>
				</li>
			</ul>
		</nav>
	</aside>

	<!-- Main Content -->
	<main class="bg-surface-900/30 overflow-auto">
		<!-- Page Route Content -->
		<div class="p-6">
			{@render children?.()}
		</div>
	</main>

	<!-- Footer (spans both columns) -->
	<footer class="bg-surface-900 text-surface-400 col-span-2 py-2 text-center text-sm">
		<div class="flex items-center justify-center gap-2">
			<span>Xivi</span>
			<span class="text-primary-400">•</span>
			<span>v0.1.0</span>
			<span class="text-primary-400">•</span>
			<a
				href="https://github.com/burmjeff/xivi/issues"
				target="_blank"
				rel="noreferrer"
				class="hover:text-primary-400 transition-colors">Report Issue</a
			>
		</div>
	</footer>
</div>
