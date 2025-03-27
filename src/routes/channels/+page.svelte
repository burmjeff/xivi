<script lang="ts">
	import Template from '@xivi/components/Template.svelte';
	import TemplateGroupList from '@xivi/components/Template_Group_List.svelte';
	import Playlist from '@xivi/components/Playlist.svelte';
	import { Tabs } from '@skeletonlabs/skeleton-svelte';

	let tabSet = $state('templates');
</script>

<div class="chManage h-full w-full items-center justify-center p-4 text-center">
	<Tabs value={tabSet} onValueChange={(e) => (tabSet = e.value)} listJustify="justify-center">
		{#snippet list()}
			<Tabs.Control value="templates">Templates</Tabs.Control>
			<Tabs.Control value="playlists">Playlists</Tabs.Control>
		{/snippet}
		{#snippet content()}
			<section class="grid h-full w-full grid-cols-2 gap-4">
				<div class="col-span-1">
					<TemplateGroupList />
				</div>
				<div class="col-span-1">
					<Tabs.Panel value="templates">
						<Template />
					</Tabs.Panel>
					<Tabs.Panel value="playlists">
						<Playlist />
					</Tabs.Panel>
				</div>
			</section>
		{/snippet}
	</Tabs>
</div>

<style lang="css">
</style>
