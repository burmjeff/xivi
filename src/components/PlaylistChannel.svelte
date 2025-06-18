<!-- PlaylistChannel.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { playlists } from '@xivi/stores/playlist_store';
	import type { PlaylistChannel } from '@xivi/data/playlist_entities';
	import {
		dndzone,
		TRIGGERS,
		SHADOW_ITEM_MARKER_PROPERTY_NAME,
		DRAGGED_ELEMENT_ID
	} from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';
	import Icon from '@iconify/svelte';

	interface Props {
		playlistId: number;
		playlistIdx: number;
		groupId: string;
		groupIdx: number;
	}

	let { playlistId, playlistIdx, groupId, groupIdx }: Props = $props();
	let dndTypeChannels = 'channels';
	let dndItem: PlaylistChannel;
	let dndIdx: number;
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;
	const dropFromOthersDisabled = true;

	const updatePlaylistChannels = async () => {
		const response = await fetch(`/api/playlist/${playlistId}/group/${groupId}/channels`);
		const data = await response.json();
		return data.playlistchannels;
	};

	onMount(async () => {
		$playlists[playlistIdx].groups[groupIdx].channels = [];
		const fetchedData = await updatePlaylistChannels();
		fetchedData.forEach(function (channel: PlaylistChannel) {
			$playlists[playlistIdx].groups[groupIdx].channels.push(channel);
			$playlists[playlistIdx].groups[groupIdx].channels =
				$playlists[playlistIdx].groups[groupIdx].channels;
		});
	});

	function handleDndConsider(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const { trigger, id } = e.detail.info;
		e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlists[playlistIdx].groups[groupIdx].channels.findIndex(
				(item) => item.id === id
			);
			dndItem = $playlists[playlistIdx].groups[groupIdx].channels[dndIdx];
			$playlists[playlistIdx].groups[groupIdx].channels = e.detail.items;
			shouldIgnoreDndEvents = true;
		} else if (!shouldIgnoreDndEvents) {
			$playlists[playlistIdx].groups[groupIdx].channels = e.detail.items;
		} else {
			$playlists[playlistIdx].groups[groupIdx].channels = [
				...$playlists[playlistIdx].groups[groupIdx].channels
			];
		}
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const { trigger, id } = e.detail.info;
		if (!shouldIgnoreDndEvents) {
			$playlists[playlistIdx].groups[groupIdx].channels = e.detail.items;
		} else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER) {
			e.detail.items = e.detail.items.filter((item) => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx, 0, dndItem);
			$playlists[playlistIdx].groups[groupIdx].channels = e.detail.items;
			shouldIgnoreDndEvents = false;
		} else {
			$playlists[playlistIdx].groups[groupIdx].channels = e.detail.items;
			shouldIgnoreDndEvents = false;
		}
	}
</script>

{#if $playlists[playlistIdx].groups[groupIdx].channels != null && $playlists[playlistIdx].groups[groupIdx].channels.length > 0}
	<table class="playlistChannel table">
		<thead>
			<tr>
				<th class="text-center">Logo</th>
				<th class="text-center">Name</th>
				<th class="text-center">tvg-id</th>
			</tr>
		</thead>
		<tbody
			use:dndzone={{
				items: $playlists[playlistIdx].groups[groupIdx].channels,
				flipDurationMs,
				type: dndTypeChannels,
				dropFromOthersDisabled
			}}
			onconsider={handleDndConsider}
			onfinalize={handleDndFinalize}
		>
			{#each $playlists[playlistIdx].groups[groupIdx].channels as channel, channelIdx (channel.id)}
				<tr id="animate" animate:flip={{ duration: flipDurationMs }}>
					<td
						><img
							class="max-h-10 max-w-16"
							src="/proxy-image?url={channel.tvg_logo}"
							alt="Logo"
						/></td
					>
					<td>{channel.title}</td>
					<td>{channel.tvg_id}</td>

					{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
						<td in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
							{channel.title}
						</td>
					{/if}
				</tr>
			{/each}
		</tbody>
	</table>
{:else}
	<div class="flex flex-col items-center justify-center px-2 py-2 text-center">
		<Icon icon="mdi:folder-off" class="text-surface-500 mb-3" width="48" height="48" />
		<h3 class="text-surface-300 mb-2 text-xl font-medium">No Playlist Channels Found</h3>
	</div>
{/if}

<style>
	/* Center all table cells and headers */
	table th,
	table td {
		text-align: center !important;
	}

	#animate {
		position: relative;
	}

	.custom-shadow-item {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		visibility: visible;
		border: 3px dashed grey;
		background: lightblue;
		opacity: 0.6;
		margin: 0;
	}
</style>
