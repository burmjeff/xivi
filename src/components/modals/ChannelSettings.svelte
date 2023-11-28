<!-- Settings.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import { templateGroups } from '@xivi/stores/template_store';
	import IconParkOutlineSaveOne from '~icons/icon-park-outline/save-one';
	import IconParkOutlineDelete from '~icons/icon-park-outline/delete';
	import type { SvelteComponent } from 'svelte';
	import { getModalStore, FileButton } from '@skeletonlabs/skeleton';
	import xivi from '$lib/assets/xivi.png';
	import type { PlaylistChannel } from '@xivi/data/playlist_entities';

	export let parent: SvelteComponent;
	const modalStore = getModalStore();
	let files: FileList;
	let groupIdx = $modalStore[0].meta.groupIdx
	let channelIdx = $modalStore[0].meta.channelIdx
	let isNew = $modalStore[0].meta.isNew
	let newImg = false

	let playlist_ch_items: PlaylistChannel[]
	let playlist_ch_matches: {
		channelname: string,
		channeltvgid: string,
		channelid: number,
		score: number
	}[]

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
		if (!isNew) {
			const fetchedItems = await updateChannelItems();
			if (typeof fetchedItems !== 'undefined') {
				playlist_ch_items = fetchedItems;
			}

			const fetchedMatches = await updateChannelMatches();
			if (typeof fetchedMatches !== 'undefined') {
				playlist_ch_matches = fetchedMatches;
			}
		}
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
				playlist_ch_items = playlist_ch_items.filter(t => t.id != playlistId)
			}
		} catch (error) {
			console.log('Error removing template channel item:', error);
			return;
		}
	}
	
</script>

{#if $modalStore[0]}
	<div class="modal-channel-settings card p-4 w-fit shadow-xl space-y-4">
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
				<table class="table table-hover text-center justify-center shadow-md">
					<thead>
						<tr class="place-self-center text-center">
							<th>Title</th>
							<th>tvg-id</th>
							<th>Remove</th>
						</tr>
					</thead>
					<tbody>
						{#if playlist_ch_items != null && playlist_ch_items.length > 0}
							{#each playlist_ch_items as channel, channelIdx (channel.id)}
								<tr class="">
									<td>{channel.title}</td>
									<td>{channel.tvg_id}</td>
									<td class="hover:bg-red-900" on:click={removeChannelItem(channel.id)}>
										<i><IconParkOutlineDelete/></i>
									</td>
								</tr>
							{/each}
						{:else}
							<p>No channels found</p>
						{/if}
					</tbody>
				</table>
				<table class="table table-hover text-center justify-center shadow-md">
					<thead>
						<tr class="center">
							<th>Title</th>
							<th>tvg-id</th>
							<th>Score</th>
						</tr>
					</thead>
					<tbody>
						{#if playlist_ch_matches != null && playlist_ch_matches.length > 0}
							{#each playlist_ch_matches as channel, channelIdx (channel.channelid)}
								<tr class="center">
									<td>{channel.channelname}</td>
									<td>{channel.channeltvgid}</td>
									<td>{channel.score}</td>
								</tr>
							{/each}
						{:else}
							<p>No channels found</p>
						{/if}
					</tbody>
				</table>
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
