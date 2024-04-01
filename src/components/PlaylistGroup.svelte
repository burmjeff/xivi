<!-- PlaylistGroup.svelte -->
<script lang="ts">
	import PlaylistChannel from './PlaylistChannel.svelte';
	import { onMount } from 'svelte';
	import { Accordion, AccordionItem, SlideToggle } from '@skeletonlabs/skeleton';
	import { playlists } from '@xivi/stores/playlist_store';
	import { getModalStore } from '@skeletonlabs/skeleton';
	import type { PlaylistGroup } from '@xivi/data/playlist_entities';
	import {
		dndzone,
		TRIGGERS,
		SHADOW_ITEM_MARKER_PROPERTY_NAME,
		DRAGGED_ELEMENT_ID
	} from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';

	export let playlistId: number;
	export let playlistIdx: number;
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;
	const dropFromOthersDisabled = true;
	let dndTypeGroups = 'groups';
	let dndItem: PlaylistGroup;
	let dndIdx: number;

	const updatePlaylistGroups = async () => {
		const response = await fetch(`/api/playlist/${playlistId}/groups`);
		const data = await response.json();
		return data.playlistgroups;
	};

	onMount(async () => {
		$playlists[playlistIdx].groups = [];
		const fetchedData = await updatePlaylistGroups();
		if (typeof fetchedData !== 'undefined') {
			fetchedData.forEach(function (group: PlaylistGroup) {
				$playlists[playlistIdx].groups.push(group);
				$playlists[playlistIdx].groups = $playlists[playlistIdx].groups;
			});
		}
	});

	async function disableGroup(group: PlaylistGroup) {
		if (group !== null) {
			try {
				const response = await fetch(`/api/playlist/group`, {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(group)
				});
				if (response.ok) {
					console.error("Updated playlist group: ", group);
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error updating playlist group:', error);
			}
		}
	}

	function handleDndConsider(e: CustomEvent<DndEvent<PlaylistGroup>>) {
		const { trigger, id } = e.detail.info;
		e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlists[playlistIdx].groups.findIndex((item) => item.id === id);
			dndItem = $playlists[playlistIdx].groups[dndIdx];
			$playlists[playlistIdx].groups = e.detail.items;
			shouldIgnoreDndEvents = true;
		} else if (!shouldIgnoreDndEvents) {
			$playlists[playlistIdx].groups = e.detail.items;
		} else {
			$playlists[playlistIdx].groups = [...$playlists[playlistIdx].groups];
		}
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<PlaylistGroup>>) {
		const { trigger, id } = e.detail.info;
		if (!shouldIgnoreDndEvents) {
			$playlists[playlistIdx].groups = e.detail.items;
		} else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER) {
			e.detail.items = e.detail.items.filter((item) => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx, 0, dndItem);
			$playlists[playlistIdx].groups = e.detail.items;
			shouldIgnoreDndEvents = false;
		} else {
			$playlists[playlistIdx].groups = e.detail.items;
			shouldIgnoreDndEvents = false;
		}
	}

	function transformDraggedElement(
		draggedEl: HTMLElement | undefined,
		data: Item | undefined,
		index: number | undefined
	) {
		data!.playlist_id = playlistId;
	}
</script>

{#if $playlists[playlistIdx].groups != null && $playlists[playlistIdx].groups.length > 0}
	<Accordion>
		<AccordionItem class="card shadow-md mb-1">
			<svelte:fragment slot="summary">
				<div class="flex flex-row items-center">
					<h4>DISABLED GROUPS</h4>
				</div>
			</svelte:fragment>
			<svelte:fragment slot="content">
				{#each $playlists[playlistIdx].groups as group, groupIdx (group.id)}
					{#if !group.enabled}
						<div class="card shadow-md p-1 px-4 flex flex-row items-center content-center">
							<span>{group.name}</span>
							<SlideToggle class="ml-auto p-1" name="group_slider" bind:checked={group.enabled} active="bg-primary-500" size="sm" on:change={() => {disableGroup(group)}}/>
						</div>
					{/if}
				{/each}
			</svelte:fragment>
		</AccordionItem>
		<section
			use:dndzone={{
				items: $playlists[playlistIdx].groups,
				flipDurationMs,
				dropFromOthersDisabled,
				type: dndTypeGroups,
				transformDraggedElement
			}}
			on:consider={handleDndConsider}
			on:finalize={handleDndFinalize}
		>
			{#each $playlists[playlistIdx].groups as group, groupIdx (group.id)}
				<div id="animate" animate:flip={{ duration: flipDurationMs }}>
					{#if group.enabled}
						<AccordionItem class="card shadow-md mb-1" key={group.id}>
							<svelte:fragment slot="summary">
								<div class="flex flex-row items-center">
									<h4>{group.name}</h4>
									<SlideToggle class="ml-auto p-1" name="group_slider" bind:checked={group.enabled} active="bg-primary-500" size="sm" on:change={() => {disableGroup(group)}}/>
								</div>
							</svelte:fragment>
							<svelte:fragment slot="content">
								<PlaylistChannel {playlistId} {playlistIdx} groupId={group.id} {groupIdx} />
							</svelte:fragment>
						</AccordionItem>
						{#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
							<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
								{group.name}
							</div>
						{/if}
					{/if}
				</div>
			{/each}
		</section>
	</Accordion>
{:else}
	<p>No playlist groups found</p>
{/if}

<style>
	#animate {
		position: relative;
		text-align: center;
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
