<!-- ViewerChannels.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Progress} from '@skeletonlabs/skeleton-svelte';
	import Player from './modals/Player.svelte';
	import { templates } from '@xivi/stores/template_store';
	import type { ViewerChannel } from '@xivi/data/viewer_entities';
	import Icon from '@iconify/svelte';

	interface Props {
		templateIdx: number;
		groupId: number;
		groupIdx: number;
	}

	let { templateIdx, groupId, groupIdx }: Props = $props();

	let playerModalOpen = $state(false);
	let currentPlayerName = $state('');
	let currentPlayerStream = $state('');

	const updateChannels = async () => {
		const response = await fetch(`/api/channels/hls/${groupId}`);
		const data = await response.json();
		return data.channels;
	};

	onMount(async () => {
		$templates[templateIdx].groups[groupIdx].viewerChannels = [];
		const fetchedData = await updateChannels();
		if (typeof fetchedData !== 'undefined') {
			fetchedData.forEach(function (channel: ViewerChannel) {
				$templates[templateIdx].groups[groupIdx].viewerChannels.push(channel);
				$templates[templateIdx].groups[groupIdx].viewerChannels =
					$templates[templateIdx].groups[groupIdx].viewerChannels;
			});
		}
	});

	function getProgress(start: string, end: string) {
		let progress = ((Date.now() - Date.parse(start)) / (Date.parse(end) - Date.parse(start))) * 100;
		return progress;
	}

	function modalPlayer(name: string, stream: string) {
		// Close modal first if it's already open to ensure proper reset
		if (playerModalOpen) {
			playerModalOpen = false;
			// Small delay to ensure modal is fully closed before reopening
			setTimeout(() => {
				currentPlayerName = name;
				currentPlayerStream = stream;
				playerModalOpen = true;
			}, 100);
		} else {
			currentPlayerName = name;
			currentPlayerStream = stream;
			playerModalOpen = true;
		}
		console.log('Opening player modal with stream:', stream);
	}

	// Modal is closed via the onOpenChange event in the Player component
</script>

<Player
	modalOpen={playerModalOpen}
	name={currentPlayerName}
	stream={currentPlayerStream}
/>

{#if $templates[templateIdx].groups[groupIdx].viewerChannels != null}
	<section class="channels grid grid-cols-2 gap-2 p-1">
		{#each $templates[templateIdx].groups[groupIdx].viewerChannels as channel (channel.id)}
			<div
				class="channel w-content max-w-content card preset-filled-primary-700-300 border border-primary-900 card-hover grid h-32 grid-cols-5"
			>
				<img class="h-auto max-h-32 w-auto self-center p-4" src={channel.logo} alt="Logo" />
				<div class="col-span-3 mb-1 ml-4 mr-4 mt-1 self-center">
					<span class="h4 font-bold text-zinc-300 drop-shadow-md">
						{channel.name}
					</span>
					<div class="text-zinc-300">
						{channel.programme}
					</div>
					{#if channel.start != '' && channel.end != ''}
						<div class="mt-2 drop-shadow-md">
							<Progress
								value={getProgress(channel.start, channel.end)}
								max={100}
							/>
						</div>
					{/if}
				</div>
				<button
					class="btn btn-md h-fit w-fit self-center"
					onclick={() => {
						modalPlayer(channel.name, channel.stream);
					}}
				>
					<Icon icon="icon-park-solid:play-two" width="50" height="50" />
				</button>
			</div>
		{/each}
	</section>
{:else}
	<p>No channels found</p>
{/if}
