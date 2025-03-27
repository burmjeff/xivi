<script lang="ts">
	import '@xivi/app.postcss';
	import { AppBar, Modal } from '@skeletonlabs/skeleton-svelte';
	import xivi from '@xivi/lib/assets/xivi.png';
	interface Props {
		children?: import('svelte').Snippet;
	}

	let { children }: Props = $props();
</script>

<!-- App Shell -->
<AppShell slotSidebarLeft="bg-surface-500/5 w-56 p-4">
	{#snippet header()}
	
			<!-- App Bar -->
			<AppBar class="h-14 justify-center">
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
		
	{/snippet}

	{#snippet sidebarLeft()}
	
			<!-- Insert the list: -->
			<nav class="list-nav">
				<ul>
					<li><a href="/">Status</a></li>
					<li><a href="/viewer">Viewer</a></li>
					<li><a href="/channels">Channel Management</a></li>
					<li><a href="/epg">Epg</a></li>
					<li><a href="/settings">Settings</a></li>
				</ul>
			</nav>
			<!-- --- -->
		
	{/snippet}

	<!-- Page Route Content -->
	{@render children?.()}
</AppShell>