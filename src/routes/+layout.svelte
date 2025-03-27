<script lang="ts">
	import '@xivi/app.css';
	import { AppBar } from '@skeletonlabs/skeleton-svelte';
	import xivi from '@xivi/lib/assets/xivi.png';
	interface Props {
		children?: import('svelte').Snippet;
	}

	let { children }: Props = $props();
</script>

<!-- Semantic layout with Tailwind grid -->
<div class="h-full grid grid-cols-[auto_1fr] grid-rows-[auto_1fr]">
	<!-- Header (spans both columns) -->
	<header class="col-span-2 sticky top-0 z-10">
		<!-- App Bar -->
		<AppBar classes="h-14 justify-center">
			{#snippet lead()}
				<img class="h-14 w-auto" src={xivi} alt="" />
			{/snippet}
			{#snippet trail()}
				<a
					class="preset-tonal-surface border border-surface-500 btn btn-sm"
					href="https://github.com/burmjeff/xivi"
					target="_blank"
					rel="noreferrer"
				>
					GitHub
				</a>
			{/snippet}
		</AppBar>
	</header>

	<!-- Sidebar -->
	<aside class="bg-surface-500/5 w-56 p-4">
		<!-- Navigation -->
		<nav class="list-nav">
			<ul>
				<li><a href="/">Status</a></li>
				<li><a href="/viewer">Viewer</a></li>
				<li><a href="/channels">Channel Management</a></li>
				<li><a href="/epg">Epg</a></li>
				<li><a href="/settings">Settings</a></li>
			</ul>
		</nav>
	</aside>

	<!-- Main Content -->
	<main>
		<!-- Page Route Content -->
		{@render children?.()}
	</main>
</div>