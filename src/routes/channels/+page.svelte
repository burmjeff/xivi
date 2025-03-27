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
			<Tabs.Panel value="templates">
				<section class="grid h-full w-full grid-cols-2 gap-4">
					<TemplateGroupList />
					<Template />
				</section>
			</Tabs.Panel>
			<Tabs.Panel value="playlists">
				<section class="grid h-full w-full grid-cols-2 gap-4">
					<TemplateGroupList />
					<Playlist />
				</section>
			</Tabs.Panel>
		{/snippet}
	</Tabs>
</div>

<style lang="css">
</style>
