<!-- PlaylistGroup.svelte -->
<script lang="ts">
	import PlaylistChannel from './PlaylistChannel.svelte';
	import { onMount } from 'svelte';
	import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
	import { playlists } from '@xivi/stores/playlist_store';
	import { getModalStore } from '@skeletonlabs/skeleton';
	import type { ModalSettings } from '@skeletonlabs/skeleton';
	import IconParkOutlineTransferData from '~icons/icon-park-outline/transfer-data';
	import type { PlaylistGroup } from '@xivi/data/playlist_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME, DRAGGED_ELEMENT_ID } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';

	export let playlistId: number;
	export let playlistIdx: number;
	const modalStore = getModalStore();
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;
	const dropFromOthersDisabled = true;
	let dndTypePlaylist = "playlist";
	let dndItem: PlaylistGroup;
	let dndIdx: number

	const updatePlaylistGroups = async () => {
		const response = await fetch(`/api/playlist/${playlistId}/groups`);
		const data = await response.json();
		return data.playlistgroups;
	};

	onMount(async () => {
		$playlists[playlistIdx].playlistGroups = [];
		const fetchedData = await updatePlaylistGroups();
		if (typeof fetchedData !== 'undefined') {
			fetchedData.forEach(function (group: PlaylistGroup) {
				$playlists[playlistIdx].playlistGroups.push(group);
				$playlists[playlistIdx].playlistGroups = $playlists[playlistIdx].playlistGroups;
			});
		}
	});

	function convertPrompt(groupId: string): void {
		const prompt: ModalSettings = {
			type: 'prompt',
			title: 'Convert Playlist Group to Template Group',
			body: 'Enter new template group name in field below.',
			value: 'Example Template',
			valueAttr: { type: 'text', minlength: 1, maxlength: 20, required: true },
			response: (groupName: string) => {
				if (groupName) convertGroup(groupName, groupId);
			},
			buttonTextCancel: 'Cancel',
			buttonTextSubmit: 'Submit'
		};
		modalStore.trigger(prompt);
	}

	async function convertGroup(groupName: string, groupId: string) {
		if (groupName !== '') {
			const newGroup = {
				name: groupName
			};
			try {
				const response = await fetch(`/api/playlist/group/${groupId}/convert`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newGroup)
				});
				const data = await response.json();
				console.log('Created template group:', data);
				//$templateGroups.push(data.templateGroup)
			} catch (error) {
				console.log('Error creating template group:', error);
				return [];
			}
		}
	}

	function handleDndConsider(e: CustomEvent<DndEvent<PlaylistGroup>>) {
		const {trigger, id} = e.detail.info;
		e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));
		
		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlists[playlistIdx].playlistGroups.findIndex(item => item.id === id);
			dndItem =  $playlists[playlistIdx].playlistGroups[dndIdx];
			$playlists[playlistIdx].playlistGroups = e.detail.items
			shouldIgnoreDndEvents = true;
		}
		else if (!shouldIgnoreDndEvents) {
            $playlists[playlistIdx].playlistGroups = e.detail.items;
        }
        else {
            $playlists[playlistIdx].playlistGroups = [...$playlists[playlistIdx].playlistGroups]
        }
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<PlaylistGroup>>) {
		const {trigger, id} = e.detail.info;
        if (!shouldIgnoreDndEvents) {
            $playlists[playlistIdx].playlistGroups = e.detail.items
        }
        else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER){
			e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx,0, dndItem)
            $playlists[playlistIdx].playlistGroups = e.detail.items
            shouldIgnoreDndEvents = false;
        }
		else {
            $playlists[playlistIdx].playlistGroups = e.detail.items
            shouldIgnoreDndEvents = false;
        }
    }
</script>

{#if $playlists[playlistIdx].playlistGroups != null && $playlists[playlistIdx].playlistGroups.length > 0}
	<Accordion>
		<section use:dndzone={{
			items: $playlists[playlistIdx].playlistGroups,
			flipDurationMs,
			dropFromOthersDisabled,
			type: dndTypePlaylist
			}} on:consider={handleDndConsider} on:finalize={handleDndFinalize}>
			{#each $playlists[playlistIdx].playlistGroups as group, groupIdx (group.id)}
			<div id="animate" animate:flip={{ duration: flipDurationMs }}>
				<AccordionItem class="card mb-1" key={group.id}>
					<svelte:fragment slot="summary"><h4>{group.name}</h4></svelte:fragment>
					<svelte:fragment slot="content">
						<PlaylistChannel {playlistIdx} groupId={group.id} {groupIdx} />
					</svelte:fragment>
				</AccordionItem>
				{#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
					<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">{group.name}</div>
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