<!-- PlaylistGroup.svelte -->
<script lang="ts">
	import PlaylistChannel from './PlaylistChannel.svelte';
	import { onMount } from 'svelte';
	import { Accordion, Switch } from '@skeletonlabs/skeleton-svelte';
	import { playlists } from '@xivi/stores/playlist_store';
	import type { PlaylistGroup } from '@xivi/data/playlist_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';
	import Icon from '@iconify/svelte';

	interface Props {
		playlistId: number;
		playlistIdx: number;
	}

	let { playlistId, playlistIdx }: Props = $props();
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;
	const dropFromOthersDisabled = true;
	let dndTypeGroups = 'groups';
	let dndItem: PlaylistGroup;
	let dndIdx: number;
	let accordionValue = $state<string[]>([]);

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
					console.error('Updated playlist group: ', group);
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
		const { trigger } = e.detail.info;
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
		_draggedEl: HTMLElement | undefined,
		data: Item | undefined,
		_index: number | undefined
	) {
		data!.playlist_id = playlistId;
	}
</script>

{#if $playlists[playlistIdx].groups != null && $playlists[playlistIdx].groups.length > 0}
	<Accordion collapsible value={accordionValue} onValueChange={(e) => (accordionValue = e.value)}>
		<div id="accord" class="playlistgroups-viewport min-w-full overflow-auto">
			<Accordion.Item value="disabled-groups" base="card shadow-md mb-1">
				{#snippet control()}
					<div class="flex w-full cursor-pointer flex-row items-center">
						<h4 class="w-full">DISABLED GROUPS</h4>
					</div>
				{/snippet}
				{#snippet panel()}
					{#each $playlists[playlistIdx].groups as group (group.id)}
						{#if !group.enabled}
							<div class="card flex w-full flex-row items-center px-4 py-1 shadow-md">
								<span class="flex-grow">{group.name}</span>
								<Switch
									name="group_enabled"
									base="flex"
									controlBase="relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
									controlActive="bg-primary-500"
									controlInactive="preset-filled-surface-200-800"
									thumbBase="pointer-events-none block h-5 w-5 rounded-full bg-white shadow-lg ring-0 transition-transform"
									thumbActive="translate-x-5"
									thumbInactive="translate-x-1"
									checked={group.enabled}
									onCheckedChange={(e) => {
										group.enabled = e.checked;
										disableGroup(group);
									}}
								/>
							</div>
						{/if}
					{/each}
				{/snippet}
			</Accordion.Item>
			<section
				use:dndzone={{
					items: $playlists[playlistIdx].groups,
					flipDurationMs,
					dropFromOthersDisabled,
					type: dndTypeGroups,
					transformDraggedElement
				}}
				onconsider={handleDndConsider}
				onfinalize={handleDndFinalize}
			>
				{#each $playlists[playlistIdx].groups as group, groupIdx (group.id)}
					<div id="animate" animate:flip={{ duration: flipDurationMs }} class="card mb-1 shadow-md">
						{#if group.enabled}
							<Accordion.Item value={group.name}>
								{#snippet control()}
									<div class="flex w-full cursor-pointer flex-row items-center">
										<h4 class="flex-grow">{group.name}</h4>
										<Switch
											name="group_enabled"
											base="flex"
											controlBase="relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
											controlActive="bg-primary-500"
											controlInactive="preset-filled-surface-200-800"
											thumbBase="pointer-events-none block h-5 w-5 rounded-full bg-white shadow-lg ring-0 transition-transform"
											thumbActive="translate-x-5"
											thumbInactive="translate-x-1"
											checked={group.enabled}
											onCheckedChange={(e) => {
												group.enabled = e.checked;
												disableGroup(group);
											}}
										/>
									</div>
								{/snippet}
								{#snippet panel()}
									<PlaylistChannel {playlistId} {playlistIdx} groupId={group.id} {groupIdx} />
								{/snippet}
							</Accordion.Item>
							{#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
								<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
									{group.name}
								</div>
							{/if}
						{/if}
					</div>
				{/each}
			</section>
		</div>
	</Accordion>
{:else}
	<div class="flex flex-col items-center justify-center px-2 py-2 text-center">
		<Icon icon="mdi:folder-off" class="text-surface-500 mb-3" width="48" height="48" />
		<h3 class="text-surface-300 mb-2 text-xl font-medium">No Playlist Groups Found</h3>
	</div>
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
