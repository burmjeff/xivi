<script lang="ts">
	import Template from '@xivi/components/Template.svelte';
	import TemplateGroupList from '@xivi/components/Template_Group_List.svelte';
	import Playlist from '@xivi/components/Playlist.svelte';
	import { Tabs } from '@skeletonlabs/skeleton-svelte';
	import Icon from '@iconify/svelte';

	let tabSet = $state('templates');
</script>

<div class="animate-fade-in">
	<!-- Page Header -->
	<header class="mb-1">
		<h1 class="text-gradient font-bold mb-1">Channel Management</h1>
		<p class="text-surface-300">Manage your templates, groups, and playlists</p>
	</header>

	<!-- Enhanced Tabs -->
	<div class="animate-slide-in">
		<Tabs
			value={tabSet}
			onValueChange={(e) => (tabSet = e.value)}
			listJustify="justify-center"
		>
			{#snippet list()}
				<div class="border-b border-surface-700/30 pb-1 gap-1 flex">
					<Tabs.Control value="templates">
						<div class="flex items-center gap-2 px-4 py-2 rounded-t-lg transition-all duration-200 hover:bg-surface-700/20 {tabSet === 'templates' ? 'bg-primary-900/30 text-primary-400 border-b-2 border-primary-500' : ''}">
							<Icon icon="mdi:file-document-outline" width="18" height="18" />
							<span>Templates</span>
						</div>
					</Tabs.Control>
					<Tabs.Control value="playlists">
						<div class="flex items-center gap-2 px-4 py-2 rounded-t-lg transition-all duration-200 hover:bg-surface-700/20 {tabSet === 'playlists' ? 'bg-primary-900/30 text-primary-400 border-b-2 border-primary-500' : ''}">
							<Icon icon="mdi:playlist-play" width="18" height="18" />
							<span>Playlists</span>
						</div>
					</Tabs.Control>
				</div>
			{/snippet}

			{#snippet content()}
				<section class="grid h-full w-full grid-cols-1 lg:grid-cols-2 gap-4">
					<div class="col-span-1">
						<div class="card bg-surface-900/30 border border-surface-700/30 rounded-lg shadow-md h-full">
							<TemplateGroupList />
						</div>
					</div>

					<div class="col-span-1">
						<div class="card bg-surface-900/30 border border-surface-700/30 rounded-lg shadow-md h-full">
							<Tabs.Panel value="templates">
								<Template />
							</Tabs.Panel>
							<Tabs.Panel value="playlists">
								<Playlist />
							</Tabs.Panel>
						</div>
					</div>
				</section>
			{/snippet}
		</Tabs>
	</div>
</div>
