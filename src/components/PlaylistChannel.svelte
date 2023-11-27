<!-- PlaylistChannel.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { playlists } from '@xivi/stores/playlist_store';
	import type { PlaylistChannel } from '@xivi/data/playlist_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME, DRAGGED_ELEMENT_ID } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';

	export let playlistIdx: number;
	export let groupId: string;
	export let groupIdx: number;
	let dndTypeChannels = "channels";
	let dndItem: PlaylistChannel;
	let dndIdx: number
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;
	const dropFromOthersDisabled = true;

	const updatePlaylistChannels = async () => {
		const response = await fetch(`/api/playlist/group/${groupId}/channels`);
		const data = await response.json();
		return data.playlistchannels;
	};

	onMount(async () => {
		$playlists[playlistIdx].groups[groupIdx].channels = [];
		const fetchedData = await updatePlaylistChannels();
        fetchedData.forEach(function (channel: PlaylistChannel) {
			$playlists[playlistIdx].groups[groupIdx].channels.push(channel);
            $playlists[playlistIdx].groups[groupIdx].channels = $playlists[playlistIdx].groups[groupIdx].channels
		});
	});

	function handleDndConsider(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const {trigger, id} = e.detail.info;
		e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlists[playlistIdx].groups[groupIdx].channels.findIndex(item => item.id === id);
			dndItem =  $playlists[playlistIdx].groups[groupIdx].channels[dndIdx];
			$playlists[playlistIdx].groups[groupIdx].channels = e.detail.items
			shouldIgnoreDndEvents = true;
		}
        else if (!shouldIgnoreDndEvents) {
            $playlists[playlistIdx].groups[groupIdx].channels = e.detail.items;
        }
        else {
            $playlists[playlistIdx].groups[groupIdx].channels = [...$playlists[playlistIdx].groups[groupIdx].channels]
        }
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const {trigger, id} = e.detail.info;
        if (!shouldIgnoreDndEvents) {
            $playlists[playlistIdx].groups[groupIdx].channels = e.detail.items
        }
        else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER){
			e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx,0, dndItem)
            $playlists[playlistIdx].groups[groupIdx].channels = e.detail.items
            shouldIgnoreDndEvents = false;
        } else {
            $playlists[playlistIdx].groups[groupIdx].channels = e.detail.items
            shouldIgnoreDndEvents = false;
        }
    }
</script>


{#if $playlists[playlistIdx].groups[groupIdx].channels != null && $playlists[playlistIdx].groups[groupIdx].channels.length > 0}
	<table class="playlistChannel table">
		<thead>
			<tr id ="thead">
				<th>Logo</th>
				<th>Name</th>
				<th>tvg-id</th>
			</tr>
		</thead>
		<tbody use:dndzone={{items: $playlists[playlistIdx].groups[groupIdx].channels, flipDurationMs, type: dndTypeChannels, dropFromOthersDisabled}} on:consider={handleDndConsider} on:finalize={handleDndFinalize}>
			{#each $playlists[playlistIdx].groups[groupIdx].channels as channel, channelIdx (channel.id)}
				<tr id="animate" animate:flip={{duration:flipDurationMs}}>
					<td><img class="w-14" src={channel.tvg_logo} alt="Logo" /></td>
					<td>{channel.title}</td>
					<td>{channel.tvg_id}</td>

					{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
						<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">{channel.title}</div>
					{/if}
				</tr>
			{/each}
		</tbody>
	</table>
{:else}
    <p>No playlist channels found</p>
{/if}

<style>
    #animate {
		position: relative;
		text-align: center;
	}
	.custom-shadow-item {
		position: absolute;
		top: 0; left: 0; right: 0; bottom: 0;
		visibility: visible;
		border: 3px dashed grey;
		background: lightblue;
		opacity: 0.6;
		margin: 0;
	}
	#thead {
		position: relative;
		text-align: center;
	}
</style>
