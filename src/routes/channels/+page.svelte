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
		<h1 class="text-gradient mb-1 text-4xl font-bold">Channel Management</h1>
		<p class="text-surface-300">Manage your templates, groups, and playlists</p>
	</header>

	<!-- Enhanced Tabs -->
	<div class="animate-slide-in relative -mt-20 pt-8">
		<Tabs value={tabSet} onValueChange={(e) => (tabSet = e.value)}>
			<Tabs.List class="border-surface-700/30 relative z-20 -mb-4 flex w-2/5 justify-end gap-1">
				<Tabs.Trigger
					value="templates"
					class="bg-surface-900/80 hover:bg-surface-700/20 data-[state=active]:bg-primary-900/50 data-[state=active]:text-primary-400 data-[state=active]:border-primary-500 flex cursor-pointer items-center gap-2 rounded-t-lg px-4 py-1 backdrop-blur-sm transition-all duration-200 data-[state=active]:border-b-2"
				>
					<Icon icon="mdi:file-document-outline" width="18" height="18" />
					Templates
				</Tabs.Trigger>
				<Tabs.Trigger
					value="playlists"
					class="bg-surface-900/80 hover:bg-surface-700/20 data-[state=active]:bg-primary-900/50 data-[state=active]:text-primary-400 data-[state=active]:border-primary-500 flex cursor-pointer items-center gap-2 rounded-t-lg px-4 py-1 backdrop-blur-sm transition-all duration-200 data-[state=active]:border-b-2"
				>
					<Icon icon="mdi:playlist-play" width="18" height="18" />
					Playlists
				</Tabs.Trigger>
			</Tabs.List>

			<div class="grid h-full w-full grid-cols-1 gap-4 lg:grid-cols-2">
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
						<Tabs.Content value="templates">
							<Template />
						</Tabs.Content>
						<Tabs.Content value="playlists">
							<Playlist />
						</Tabs.Content>
					</div>
				</div>
			</div>
		</Tabs>
	</div>
</div>
