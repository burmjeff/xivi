<script lang="ts">
	import Template from '@xivi/components/Template.svelte';
	import TemplateGroupList from '@xivi/components/Template_Group_List.svelte';
	import Playlist from '@xivi/components/Playlist.svelte';
	import { Tabs } from '@skeletonlabs/skeleton-svelte';
	import Icon from '@iconify/svelte';

	let tabSet = $state('templates');
</script>

<div class="animate-fade-in relative">
	<!-- Page Header -->
	<header class="relative z-10 mb-2">
		<h1 class="text-gradient mb-1 font-bold text-4xl">Channel Management</h1>
		<p class="text-surface-300">Manage your templates, groups, and playlists</p>
	</header>

	<!-- Enhanced Tabs -->
	<div class="animate-slide-in relative -mt-20 pt-8">
		<Tabs value={tabSet} onValueChange={(e) => (tabSet = e.value)} listJustify="justify-end">
			{#snippet list()}
				<div class="w-2/5 border-surface-700/30 flex gap-1 -mb-4 relative z-20">
					<Tabs.Control
						value="templates"
						base="flex items-center gap-2 rounded-t-lg px-4 py-1 transition-all duration-200 backdrop-blur-sm cursor-pointer"
						stateInactive="bg-surface-900/80 hover:bg-surface-700/20"
						stateActive="bg-primary-900/50 text-primary-400 border-primary-500 border-b-2"
					>
						{#snippet lead()}
							<Icon icon="mdi:file-document-outline" width="18" height="18" />
						{/snippet}
						Templates
					</Tabs.Control>
					<Tabs.Control
						value="playlists"
						base="flex items-center gap-2 rounded-t-lg px-4 py-1 transition-all duration-200 backdrop-blur-sm cursor-pointer"
						stateInactive="bg-surface-900/80 hover:bg-surface-700/20"
						stateActive="bg-primary-900/50 text-primary-400 border-primary-500 border-b-2"
					>
						{#snippet lead()}
							<Icon icon="mdi:playlist-play" width="18" height="18" />
						{/snippet}
						Playlists
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
