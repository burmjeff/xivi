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
		<h1 class="text-gradient mb-1 font-bold">Channel Management</h1>
		<p class="text-surface-300">Manage your templates, groups, and playlists</p>
	</header>

	<!-- Enhanced Tabs -->
	<div class="animate-slide-in">
		<Tabs value={tabSet} onValueChange={(e) => (tabSet = e.value)} listJustify="justify-center">
			{#snippet list()}
				<div class="border-surface-700/30 flex gap-1 border-b pb-1">
					<Tabs.Control value="templates">
						<div
							class="hover:bg-surface-700/20 flex items-center gap-2 rounded-t-lg px-4 py-2 transition-all duration-200 {tabSet ===
							'templates'
								? 'bg-primary-900/30 text-primary-400 border-primary-500 border-b-2'
								: ''}"
						>
							<Icon icon="mdi:file-document-outline" width="18" height="18" />
							<span>Templates</span>
						</div>
					</Tabs.Control>
					<Tabs.Control value="playlists">
						<div
							class="hover:bg-surface-700/20 flex items-center gap-2 rounded-t-lg px-4 py-2 transition-all duration-200 {tabSet ===
							'playlists'
								? 'bg-primary-900/30 text-primary-400 border-primary-500 border-b-2'
								: ''}"
						>
							<Icon icon="mdi:playlist-play" width="18" height="18" />
							<span>Playlists</span>
						</div>
					</Tabs.Control>
				</div>
			{/snippet}

			{#snippet content()}
				<section class="grid h-full w-full grid-cols-1 gap-4 lg:grid-cols-2">
					<div class="col-span-1">
						<div
							class="card bg-surface-900/30 border-surface-700/30 h-full rounded-lg border shadow-md"
						>
							<TemplateGroupList />
						</div>
					</div>

					<div class="col-span-1">
						<div
							class="card bg-surface-900/30 border-surface-700/30 h-full rounded-lg border shadow-md"
						>
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
