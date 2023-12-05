<!-- Settings.svelte -->

<script lang="ts">
	import { onMount, type SvelteComponent } from 'svelte';
	import { templateGroups } from '@xivi/stores/template_store';
	import IconParkOutlineSaveOne from '~icons/icon-park-outline/save-one';
	import IconParkOutlineDelete from '~icons/icon-park-outline/delete';
	import { writable } from 'svelte/store';
	import { getModalStore, FileButton } from '@skeletonlabs/skeleton';
	import xivi from '$lib/assets/xivi.png';
	import type { Match, PlaylistChannel } from '@xivi/data/playlist_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
    import {flip} from 'svelte/animate';
    import {fade} from 'svelte/transition';
    import {cubicIn} from 'svelte/easing';
	import { playlistMatches } from '@xivi/stores/playlist_store';

	export let parent: SvelteComponent;

	const playlist_ch_items = writable<PlaylistChannel[]>([]);

	const modalStore = getModalStore();
	let files: FileList;
	let groupIdx = $modalStore[0].meta.groupIdx
	let channelIdx = $modalStore[0].meta.channelIdx
	let isNew = $modalStore[0].meta.isNew
	let newImg = false

	let dndTypeChannels = "channelSettings";
	let shouldIgnoreDndEvents = false;
	const dropFromOthersDisabled = true;
    const flipDurationMs = 150;
    let dndItem: any;
	let dndIdx: number;

	let formData: {
		id: number,
		name: string,
		tvgid: string,
		logoid: number,
		logo: string
	}

	if (!isNew) {
		formData = {
				id: Number($templateGroups[groupIdx].channels[channelIdx].id),
				name: $templateGroups[groupIdx].channels[channelIdx].name,
				tvgid: $templateGroups[groupIdx].channels[channelIdx].tvgid,
				logoid: $templateGroups[groupIdx].channels[channelIdx].logoid,
				logo: $templateGroups[groupIdx].channels[channelIdx].logo
			};
	} else {
		formData = {
			id: 0,
			name: "",
			tvgid: "",
			logoid: 0,
			logo: xivi
		}
		//newImg = true
	}

	const updateChannelItems = async () => {
		const response = await fetch(`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}/items`);
		const data = await response.json();
		console.log(data)
		return data.playlistchannels;
	};

	const updateChannelMatches = async () => {
		const response = await fetch(`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}/matches`);
		const data = await response.json();
		console.log(data)
		return data.vectormatches;
	};

	onMount(async () => {
		//if (!isNew) {
			const fetchedItems = await updateChannelItems();
			if (typeof fetchedItems !== 'undefined') {
				playlist_ch_items.set(fetchedItems);
			}

			const fetchedMatches = await updateChannelMatches();
			if (typeof fetchedMatches !== 'undefined') {
				playlistMatches.set(fetchedMatches);
			}
		//}
	});

	const toBase64 = (file: File) =>
		new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.readAsDataURL(file);
			reader.onload = () => resolve(reader.result);
			reader.onerror = reject;
		});

	const uploadImage = async () => {
		const response = await fetch(`/api/logo`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json'
			},
			body: JSON.stringify({image: formData.logo})
		});
		const data = await response.json();
		return data.logo;
	}

	async function onUploadHandler(e: Event) {
		console.log('file data:', e);
		if (files) {
			const result = String(await toBase64(files[0]));
			if (result) {
				formData.logo = result
				newImg = true
			}
		}
	}

	async function onFormSubmit(): Promise<void> {
		if (newImg) {
			const fetchedData = await uploadImage();
			formData.logoid = fetchedData.id;
			formData.logo = fetchedData.image;
		}
		if ($modalStore[0].response) $modalStore[0].response(formData);
		modalStore.close();
	}

	async function deleteChannel() {
		try {
			const response = await fetch(`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted template channel:', data);
			$templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels.filter(t => Number(t.id) != formData.id)
			modalStore.close();
		} catch (error) {
			console.log('Error deleting template channel:', error);
			return;
		}
	}

	async function removeChannelItem(playlistId: string) {
			let channelItem = {
				channel_id: $templateGroups[groupIdx].channels[channelIdx].id,
				playlist_channel_id: playlistId
			};
		try {
			const response = await fetch(`/api/template/channel/item`, {
				method: 'DELETE',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(channelItem)
				});
			const data = await response.status;
			if (response.ok) {
				console.log('Removed template channel item:', data);
				$playlist_ch_items = $playlist_ch_items.filter(t => t.id != playlistId)
			}
		} catch (error) {
			console.log('Error removing template channel item:', error);
			return;
		}
	}

	function handleDndConsiderMatch(e: CustomEvent<DndEvent<Match>>) {
		const {trigger, id} = e.detail.info;
		e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlistMatches.findIndex(item => item.id === Number(id));
			dndItem =  $playlistMatches[dndIdx];
			$playlistMatches = e.detail.items
			shouldIgnoreDndEvents = true;
		}
        else if (!shouldIgnoreDndEvents) {
            $playlistMatches = e.detail.items;
        }
        else {
            $playlistMatches = [...$playlistMatches]
        }
	}
	function handleDndConsiderItem(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const {trigger, id} = e.detail.info;
		//e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlist_ch_items.findIndex(item => item.id === id);
			dndItem =  $playlist_ch_items[dndIdx];
			$playlist_ch_items = e.detail.items
			shouldIgnoreDndEvents = true;
		}
        else if (!shouldIgnoreDndEvents) {
            $playlist_ch_items = e.detail.items;
        }
        else {
			$playlist_ch_items = e.detail.items;
        }
	}
	function handleDndFinalizeMatch(e: CustomEvent<DndEvent<Match>>) {
		const {trigger, id} = e.detail.info;
        if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreDndEvents) {
            //e.detail.items = e.detail.items.filter(item => !item.isDragged);
            $playlistMatches = e.detail.items
            shouldIgnoreDndEvents = false;
        }
        else if (!shouldIgnoreDndEvents) {
            $playlistMatches = e.detail.items
        }
        else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER){
			e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx,0, dndItem)
            $playlistMatches = e.detail.items
            shouldIgnoreDndEvents = false;
        } else {
            $playlistMatches = e.detail.items
            shouldIgnoreDndEvents = false;
        }
    }
	function handleDndFinalizeItem(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const {trigger, id} = e.detail.info;
        if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreDndEvents) {
            e.detail.items = e.detail.items.filter(item => !item.isDragged);
            $playlist_ch_items = e.detail.items
            shouldIgnoreDndEvents = false;
        }
        else if (!shouldIgnoreDndEvents) {
            $playlist_ch_items = e.detail.items
        } else {
            $playlist_ch_items = e.detail.items
            shouldIgnoreDndEvents = false;
        }
    }
</script>

{#if $modalStore[0]}
	<div class="modal-channel-settings card p-4 shadow-xl space-y-4 max-w-screen max-h-screen">
		{#if isNew}
			<header class="text-2xl font-bold text-center justify-center">Add Channel</header>
		{:else}
			<header class="text-2xl font-bold text-center justify-center">Channel Settings</header>
		{/if}
		<form class="modal-form border border-surface-500 p-4 space-y-4 rounded-container-token">
			<label class="channel_name">
				<span>Channel Name</span>
				<input
					class="input variant-form-material"
					type="text"
					bind:value={formData.name}
					placeholder=""
				/>
			</label>
			<label class="channel_tvgid">
				<span>Channel tvgid</span>
				<input
					class="input variant-form-material"
					type="text"
					bind:value={formData.tvgid}
					placeholder=""
				/>
			</label>
			<div class="channel_logo">
				<span>Channel Logo</span>
				<div class="grid grid-cols-2 p-1 w-64 items-center space-x-10">
					<img class="w-fit" src={formData.logo} alt="Logo" />
					<FileButton
						name="files"
						bind:files
						accept=".png,.jpg,.webp,.svg"
						on:change={onUploadHandler}>Upload</FileButton>
				</div>
			</div>
			<div class="playlist_ch_items grid grid-cols-2 space-x-2">
				<div class="max-h-80 overflow-y-scroll">
					<table class="table table-hover text-center justify-center shadow-md" use:dndzone={{items: $playlist_ch_items, flipDurationMs, type: dndTypeChannels}} on:consider={handleDndConsiderItem} on:finalize={handleDndFinalizeItem}>
						<thead>
							<tr id ="thead">
								<th>Title</th>
								<th>tvg-id</th>
								<th>Remove</th>
							</tr>
						</thead>
						<tbody>
							{#if $playlist_ch_items != null && $playlist_ch_items.length > 0}
								{#each $playlist_ch_items as channel, channelIdx (channel.id)}
									<tr id="animate" animate:flip={{duration:flipDurationMs}}>
										<td>{channel.title}</td>
										<td>{channel.tvg_id}</td>
										<td class="hover:bg-red-900 w-5" on:click={removeChannelItem(channel.id)}>
											<i><IconParkOutlineDelete/></i>
										</td>

										{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
											<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">{channel.title}</div>
										{/if}
									</tr>
								{/each}
							{:else}
								<p>No channels found</p>
							{/if}
						</tbody>
					</table>
				</div>
				<div class="max-h-80 overflow-y-scroll">
					<table class="table table-hover text-center justify-center shadow-md ">
						<thead>
							<tr id ="thead">
								<th>Title</th>
								<th>tvg-id</th>
								<th>Score</th>
							</tr>
						</thead>
						<tbody use:dndzone={{items: $playlistMatches, flipDurationMs, type: dndTypeChannels, dropFromOthersDisabled}} on:consider={handleDndConsiderMatch} on:finalize={handleDndFinalizeMatch}>
							{#if $playlistMatches != null && $playlistMatches.length > 0}
								{#each $playlistMatches as channel, channelIdx (channel.id)}
									<tr id="animate" animate:flip={{duration:flipDurationMs}}>
										<td>{channel.name}</td>
										<td>{channel.tvgid}</td>
										<td>{channel.score}</td>

										{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
											<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">{channel.name}</div>
										{/if}
									</tr>
								{/each}
							{:else}
								<p>No channels found</p>
							{/if}
					</table>
				</div>
			</div>
		</form>
		<footer class="modal-footer {parent.regionFooter}">
			{#if !isNew}
				<button class="btn variant-ghost-error" on:click={deleteChannel}>
					<i><IconParkOutlineDelete/></i>
					<span>Delete</span>
				</button>
			{/if}
			<button class="btn {parent.buttonNeutral}" on:click={parent.onClose}>
				{parent.buttonTextCancel}</button>
			<button class="btn {parent.buttonPositive}" on:click={onFormSubmit}>
				<i><IconParkOutlineSaveOne/></i>
				<span>Save Changes</span>
			</button>
			
		</footer>
	</div>
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
	.center { 
		text-align: center; 
		justify-content: center;
		align-items: center;
		width: 100%; 
	} 
</style>
