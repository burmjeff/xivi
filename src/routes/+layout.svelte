<script lang="ts">
	import '@xivi/app.postcss';
	import { AppShell, AppBar, Modal, type ModalComponent } from '@skeletonlabs/skeleton';
	import { Avatar } from '@skeletonlabs/skeleton';
	import { computePosition, autoUpdate, flip, shift, offset, arrow } from '@floating-ui/dom';
	import { storePopup } from '@skeletonlabs/skeleton';
	import { initializeStores } from '@skeletonlabs/skeleton';
	import ChannelSettings from '@xivi/components/modals/ChannelSettings.svelte';
	import GroupSettings from '@xivi/components/modals/GroupSettings.svelte';
	import PlaylistSettings from '@xivi/components/modals/PlaylistSettings.svelte';
	import Player from '@xivi/components/modals/Player.svelte';
	import xivi from '@xivi/lib/assets/xivi.png';

	initializeStores();

	const modalRegistry: Record<string, ModalComponent> = {
		// Set a unique modal ID, then pass the component reference
		modalChannelSettings: { ref: ChannelSettings },
		modalGroupSettings: { ref: GroupSettings },
		modalPlaylistSettings: { ref: PlaylistSettings },
		modalPlayer: { ref: Player }
	};

	storePopup.set({ computePosition, autoUpdate, flip, shift, offset, arrow });
</script>

<!-- App Shell -->
<AppShell slotSidebarLeft="bg-surface-500/5 w-56 p-4">
	<svelte:fragment slot="header">
		<!-- App Bar -->
		<AppBar class="h-14 justify-center">
			<svelte:fragment slot="lead">
				<img class="h-14 w-auto" src={xivi} alt="" />
			</svelte:fragment>
			<svelte:fragment slot="trail">
				<a
					class="variant-ghost-surface btn btn-sm"
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
				<li><a href="/">Status</a></li>
				<li><a href="/viewer">Viewer</a></li>
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
