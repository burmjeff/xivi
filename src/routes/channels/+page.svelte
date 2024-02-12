<script lang="ts">
	import Template from '@xivi/components/Template.svelte';
	import TemplateGroupList from '@xivi/components/Template_Group_List.svelte';
	import Playlist from '@xivi/components/Playlist.svelte';
	import { TabGroup, Tab } from '@skeletonlabs/skeleton';

	let tabSet: number = 0;
</script>

<div class="chManage h-full w-full items-center justify-center p-4 text-center">
	<TabGroup justify="justify-center">
		<Tab bind:group={tabSet} name="tab1" value={0}>Templates</Tab>
		<Tab bind:group={tabSet} name="tab2" value={1}>Playlists</Tab>
		<!-- Tab Panels --->
		<svelte:fragment slot="panel">
			<section class="grid h-full w-full grid-cols-2 gap-4">
				<TemplateGroupList />
				{#if tabSet === 0}
					<Template />
				{:else if tabSet === 1}
					<Playlist />
				{/if}
			</section>
		</svelte:fragment>
	</TabGroup>
</div>

<style lang="postcss">
</style>
