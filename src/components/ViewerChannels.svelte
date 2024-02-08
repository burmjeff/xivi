<!-- Viewer.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import {
        ProgressBar,
		popup,
		getModalStore,
		type ModalSettings,
		type PopupSettings
	} from '@skeletonlabs/skeleton';
	import { templates } from '@xivi/stores/template_store';
    import { channels } from '@xivi/stores/viewer_store';
    import type { ViewerChannel } from '@xivi/data/viewer_entities';
	import Icon from '@iconify/svelte';

    export let templateIdx: number;
    export let groupId: string;
	export let groupIdx: number;

    const modalStore = getModalStore();

	const updateChannels = async () => {
        const response = await fetch(`/api/live/channels/${groupId}`);
        const data = await response.json();
        return data.channels;
    }

    onMount(async () => {
        $templates[templateIdx].groups[groupIdx].viewerChannels = [];
        const fetchedData = await updateChannels()
        if (typeof fetchedData !== 'undefined') {
			fetchedData.forEach(function (channel: ViewerChannel) {
				$templates[templateIdx].groups[groupIdx].viewerChannels.push(channel);
				$templates[templateIdx].groups[groupIdx].viewerChannels = $templates[templateIdx].groups[groupIdx].viewerChannels
			});
		}
    });

    function getProgress(start: string, end: string){
        let progress = (Date.now() - Date.parse(start)) / (Date.parse(end) - Date.parse(start)) * 100
        return progress

    }

    function modalPlayer(name: string, stream: string) {
		new Promise<boolean>((resolve) => {
			const modal: ModalSettings = {
				type: 'component',
				component: 'modalPlayer',
				meta: { 
                    name: name,
					stream: stream
				 },
				response: (r: boolean) => {
					resolve(r);
				}
			};
			modalStore.trigger(modal);
		}).then((r: any) => {
		});
	}

</script>

{#if $templates[templateIdx].groups[groupIdx].viewerChannels != null}
    <section class="channels p-1 grid grid-cols-2 gap-2">
            {#each $templates[templateIdx].groups[groupIdx].viewerChannels as channel, channelIdx (channel.id)}
                <div class="channel grid grid-cols-5 variant-ghost-tertiary card card-hover h-32 w-content max-w-content">
                    <img class="h-auto max-h-32 w-auto self-center p-4" src={channel.logo} alt="Logo" />
                    <div class="col-span-3 mt-1 mb-1 ml-4 mr-4 self-center">
                        <span class="h4 font-bold text-zinc-300 drop-shadow-md">
                            {channel.name}
                        </span>
                        <div class="text-zinc-300">
                            {channel.programme}
                        </div>
                        {#if channel.start != "" && channel.end != ""}
                            <ProgressBar label="Progress Bar" class="mt-2 drop-shadow-md" value={getProgress(channel.start, channel.end)} max={100} />
                        {/if}
                    </div>
                    <button class="btn btn-md w-fit h-fit self-center" on:click={() => {modalPlayer(channel.name, channel.stream);}}>
                        <Icon icon="icon-park-solid:play-two" width="50" height="50"/>
                    </button>
                </div>
            {/each}
        
    </section>
{:else}
        <p>No channels found</p>
{/if}