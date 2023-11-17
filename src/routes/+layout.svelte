<script lang="ts">
	import '@xivi/app.postcss';
	import { AppShell, AppBar, Modal, type ModalComponent } from '@skeletonlabs/skeleton';
	import { Avatar } from '@skeletonlabs/skeleton';
	import { computePosition, autoUpdate, flip, shift, offset, arrow } from '@floating-ui/dom';
	import { storePopup } from '@skeletonlabs/skeleton';
	import { initializeStores } from '@skeletonlabs/skeleton';
	import ChannelSettings from '@xivi/components/modals/ChannelSettings.svelte';
	import xivi from '$lib/assets/xivi.png';

	initializeStores();

	const modalRegistry: Record<string, ModalComponent> = {
	// Set a unique modal ID, then pass the component reference
	modalChannelSettings: { ref: ChannelSettings },
	};

	storePopup.set({ computePosition, autoUpdate, flip, shift, offset, arrow });
</script>

<!-- App Shell -->
<AppShell slotSidebarLeft="bg-surface-500/5 w-56 p-4">
	<svelte:fragment slot="header">
		<!-- App Bar -->
		<AppBar>
			<svelte:fragment slot="lead">
				<img class="h-12 w-auto" src={xivi}  alt=""/>
			</svelte:fragment>
			<svelte:fragment slot="trail">
				<a
					class="btn btn-sm variant-ghost-surface"
					href="https://github.com/burmjeff/xivi"
					target="_blank"
					rel="noreferrer"
				>
					GitHub
				</a>
			</svelte:fragment>
		</AppBar>
	</svelte:fragment>
	
	<svelte:fragment slot="sidebarLeft">
		<!-- Insert the list: -->
		<nav class="list-nav">
			<ul>
				<li><a href="/">Home</a></li>
				<li><a href="/templates">Template Management</a></li>
				<li><a href="/channels">Channel Management</a></li>
				<li><a href="/epg">Epg</a></li>
				<li><a href="/settings">Settings</a></li>
			</ul>
		</nav>
		<!-- --- -->
	</svelte:fragment>

	<!-- Page Route Content -->
	<slot />
</AppShell>
<Modal components={modalRegistry} />