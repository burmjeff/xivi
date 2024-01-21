<script lang="ts">
    import Template from '@xivi/components/Template.svelte';
    import TemplateGroupList from '@xivi/components/Template_Group_List.svelte';
    import Playlist from '@xivi/components/Playlist.svelte';
    import { TabGroup, Tab } from '@skeletonlabs/skeleton';

    let tabSet: number = 0;
</script>

<div class="chManage p-4 text-center justify-center items-center h-full w-full">
    <TabGroup justify="justify-center">
        <Tab bind:group={tabSet} name="tab1" value={0}>Templates</Tab>
        <Tab bind:group={tabSet} name="tab2" value={1}>Playlists</Tab>
        <!-- Tab Panels --->
        <svelte:fragment slot="panel">
            <section class="grid grid-cols-2 gap-4 h-full w-full">
                <TemplateGroupList/>
                {#if tabSet === 0}
                    <Template/>
                {:else if tabSet === 1}
                    <Playlist/>
                {/if}
            </section>
        </svelte:fragment>
    </TabGroup>
</div>

<style lang="postcss">
	@keyframes glow {
		0% {
			@apply bg-primary-400/50;
		}
		33% {
			@apply bg-secondary-400/50;
		}
		66% {
			@apply bg-tertiary-400/50;
		}
		100% {
			@apply bg-primary-400/50;
		}
	}
	@keyframes pulse {
		50% {
			transform: scale(1.5);
		}
	}

	.hovering {
        border-color: orange;
    }
    .item {
        display: inline; /* required for flip to work */
    }
    li {
        background-color: lightgray;
        cursor: pointer;
        display: inline-block;
        margin-right: 10px;
        padding: 10px;
    }
    li:hover {
        background: orange;
        color: white;
    }
  p {
        border: solid lightgray 1px;
        display: flex; /* required for drag & drop to work when .item display is inline */
        height: 40px; /* needed when empty */
        padding: 10px;
    }
</style>
