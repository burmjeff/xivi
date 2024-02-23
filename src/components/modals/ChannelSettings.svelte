<!-- ChannelSettings.svelte -->

<script lang="ts">
	import { onMount, type SvelteComponent } from 'svelte';
	import { templateGroups } from '@xivi/stores/template_store';
	import Icon from '@iconify/svelte';
	import { writable } from 'svelte/store';
	import { getModalStore, FileButton, popup, InputChip, Autocomplete, type AutocompleteOption, type PopupSettings } from '@skeletonlabs/skeleton';
	import xivi from '$lib/assets/xivi.png';
	import type { Match, PlaylistChannel } from '@xivi/data/playlist_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';
	import { playlistMatches } from '@xivi/stores/playlist_store';
	import { logos } from '@xivi/stores/logo_store';
	import type { Logo } from '@xivi/data/logo_entities';

	export let parent: SvelteComponent;

	const playlist_ch_items = writable<PlaylistChannel[]>([]);

	const modalStore = getModalStore();
	let files: FileList;
	let groupIdx = $modalStore[0].meta.groupIdx;
	let channelIdx = $modalStore[0].meta.channelIdx;
	let isNew = $modalStore[0].meta.isNew;
	let newImg = false;

	let tvgidList: string[];
	let tvgidInputList: string[];
	let inputTvgid: string;
	let tvgidOptions: AutocompleteOption<string>[];

	let dndTypeChannels = 'channelSettings';
	let shouldIgnoreMatchEvents = false;
	let shouldIgnoreItemEvents = false;
	const dropFromOthersDisabled = true;
	const flipDurationMs = 150;
	let dndItem: any;
	let dndIdx: number;

	let formData: {
		id: number;
		name: string;
		tvgid: string;
		logoid: number;
		logo: string;
	};

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
			name: '',
			tvgid: '',
			logoid: 0,
			logo: xivi
		};
		//newImg = true
	}

	const updateChannelItems = async () => {
		const response = await fetch(
			`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}/items`
		);
		const data = await response.json();
		return data.playlistchannels;
	};

	const updateChannelMatches = async () => {
		const response = await fetch(
			`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}/matches`
		);
		const data = await response.json();
		return data.vectormatches;
	};

	const updateLogos = async () => {
		const response = await fetch(`/api/logos`);
		const data = await response.json();
		if (typeof data.logos !== 'undefined') {
			logos.set(data.logos);
		}
	};

	const updateTvgids = async () => {
		const response = await fetch(
			`/api/epg/tvgids`
		);
		const data = await response.json();
		return data.tvgids;
	};

	let popupTvgid: PopupSettings = {
		event: 'focus-click',
		target: 'popupTvgid',
		placement: 'bottom'
	};

	onMount(async () => {
		const fetchedTvgids = await updateTvgids();
		if (typeof fetchedTvgids !== 'undefined') {
			tvgidList = fetchedTvgids;
			tvgidOptions = tvgidList.map((tvgid) => {
				return {
					label: `${tvgid}`,
					value: `${tvgid}`,
					meta: `${tvgid}`
				};
			});
		}
		if (formData.tvgid != "") {
			tvgidInputList[0] = formData.tvgid
		}

		if (!isNew) {
			const fetchedItems = await updateChannelItems();
			if (typeof fetchedItems !== 'undefined') {
				playlist_ch_items.set(fetchedItems);
			}

			const fetchedMatches = await updateChannelMatches();
			if (typeof fetchedMatches !== 'undefined') {
				playlistMatches.set(fetchedMatches);
			}
		}
	});

	const popupLogo: PopupSettings = {
		// Represents the type of event that opens/closed the popup
		event: 'click',
		// Matches the data-popup value on your popup element
		target: 'popupLogo',
		// Defines which side of your trigger the popup will appear
		placement: 'right',
		closeQuery: '#chooseImage'
	};

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
			body: JSON.stringify({ image: formData.logo })
		});
		const data = await response.json();
		return data.logo;
	};

	async function onUploadHandler(e: Event) {
		if (files) {
			const result = String(await toBase64(files[0]));
			if (result) {
				formData.logo = result;
				newImg = true;
			}
		}
	}

	async function chooseImage(logo: Logo) {
		formData.logoid = logo.id;
		formData.logo = logo.image;
	}

	function onInputChipSelect(event: CustomEvent<AutocompleteOption<string>>): void {
		if (tvgidInputList.length === 0) {
			tvgidInputList.push(event.detail.label);
			tvgidInputList = [...tvgidInputList];
		}
	}

	async function onFormSubmit(): Promise<void> {
		if (newImg) {
			const fetchedData = await uploadImage();
			formData.logoid = fetchedData.id;
			formData.logo = fetchedData.image;
		}
		formData.tvgid = tvgidInputList[0]
		if ($modalStore[0].response) $modalStore[0].response(formData);
		modalStore.close();
	}

	async function deleteChannel() {
		try {
			const response = await fetch(
				`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}`,
				{
					method: 'DELETE'
				}
			);
			const data = await response.status;
			console.log('Deleted template channel:', data);
			$templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels.filter(
				(t) => Number(t.id) != formData.id
			);
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
			const response = await fetch(`/api/template/chan/item`, {
				method: 'DELETE',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(channelItem)
			});
			const data = await response.status;
			if (response.ok) {
				console.log('Removed template channel item:', data);
				$playlist_ch_items = $playlist_ch_items.filter((t) => t.id != playlistId);
			}
		} catch (error) {
			console.log('Error removing template channel item:', error);
			return;
		}
	}

	async function addChannelMatch(channelId: string) {
		try {
			const response = await fetch(`/api/template/channel/${formData.id}/match/${channelId}`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				}
			});
			if (response.ok) {
				const data = await response.json();
				console.log('Add Channel Match:', data);
				$playlist_ch_items.push(data.playlistchannel);
				$playlist_ch_items = [...$playlist_ch_items];
			} else {
				console.error('Error:', response.status, response.statusText);
			}
		} catch (error) {
			console.log('Error creating template group:', error);
		}
	}

	function handleDndConsiderMatch(e: CustomEvent<DndEvent<Match>>) {
		const { trigger, id } = e.detail.info;
		//e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlistMatches.findIndex((item) => item.id === Number(id));
			dndItem = $playlistMatches[dndIdx];
			$playlistMatches = e.detail.items;
			shouldIgnoreMatchEvents = true;
		} else if (!shouldIgnoreMatchEvents) {
			$playlistMatches = e.detail.items;
		} else {
			$playlistMatches = [...$playlistMatches];
		}
	}
	function handleDndConsiderItem(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const { trigger, id } = e.detail.info;
		//e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $playlist_ch_items.findIndex((item) => item.id === id);
			dndItem = $playlist_ch_items[dndIdx];
			$playlist_ch_items = e.detail.items;
			shouldIgnoreItemEvents = true;
		} else if (!shouldIgnoreItemEvents) {
			$playlist_ch_items = e.detail.items;
		} else {
			$playlist_ch_items = [...$playlist_ch_items];
		}
	}
	function handleDndFinalizeMatch(e: CustomEvent<DndEvent<Match>>) {
		const { trigger, id } = e.detail.info;
		if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreMatchEvents) {
			//e.detail.items = e.detail.items.filter(item => !item.isDragged);
			$playlistMatches = e.detail.items;
			shouldIgnoreMatchEvents = false;
		} else if (!shouldIgnoreMatchEvents) {
			$playlistMatches = e.detail.items;
		} else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER) {
			e.detail.items = e.detail.items.filter((item) => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx, 0, dndItem);
			$playlistMatches = e.detail.items;
			shouldIgnoreMatchEvents = false;
		} else {
			$playlistMatches = e.detail.items;
			shouldIgnoreMatchEvents = false;
		}
	}
	function handleDndFinalizeItem(e: CustomEvent<DndEvent<PlaylistChannel>>) {
		const { trigger, id } = e.detail.info;
		if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreItemEvents) {
			e.detail.items = e.detail.items.filter((item) => !item.isDragged);
			$playlist_ch_items = e.detail.items;
			addChannelMatch(id);
			shouldIgnoreItemEvents = false;
		} else if (!shouldIgnoreItemEvents) {
			$playlist_ch_items = e.detail.items;
		} else {
			$playlist_ch_items = e.detail.items;
			shouldIgnoreItemEvents = false;
		}
	}

	function transformDraggedElement(
		draggedEl: HTMLElement | undefined,
		data: Item | undefined,
		index: number | undefined
	) {
		if (!shouldIgnoreItemEvents) data!.isDragged = true;
	}
</script>

{#if $modalStore[0]}
	<div class="modal-channel-settings max-w-screen card max-h-screen space-y-2 p-2 shadow-xl">
		{#if isNew}
			<header class="justify-center text-center text-2xl font-bold">Add Channel</header>
		{:else}
			<header class="justify-center text-center text-2xl font-bold">Channel Settings</header>
		{/if}
		<form class="modal-form space-y-4 border border-surface-500 p-4 rounded-container-token">
			<div class="playlist_ch_items grid grid-cols-5 space-x-6">
				<div class="form col-span-2">
					<label class="channel_name">
						<span>Channel Name</span>
						<input
							class="input variant-form-material"
							type="text"
							bind:value={formData.name}
							placeholder=""
						/>
					</label>
					<div class="channel_tvgid" use:popup={popupTvgid}>
						<span>Channel tvgid</span>
						<InputChip
								bind:input={inputTvgid}
								bind:value={tvgidInputList}
								max={1}
								placeholder="Add a tvg_id..."
								name="chips"
						/>
					</div>
					<div class="channel_logo">
						<div class="grid w-64 grid-cols-2 items-center space-x-10 p-1">
							<img class="w-fit" src={formData.logo} alt="Logo" />
							<button
								class="variant-filled-primary btn h-fit w-fit"
								on:click={updateLogos}
								use:popup={popupLogo}>Choose Logo</button
							>
						</div>
					</div>
				</div>
				<div class="max-h-72 col-span-3">
					<header class="justify-center text-center font-bold mb-2">Current Playlist Channels</header>
					<div class="max-h-72 overflow-y-scroll">
						<table class="table table-hover justify-center text-center shadow-md">
							<thead>
								<tr id="thead">
									<th>Title</th>
									<th>tvg-id</th>
									<th>Remove</th>
								</tr>
							</thead>
							<tbody
								use:dndzone={{
									items: $playlist_ch_items,
									flipDurationMs,
									type: dndTypeChannels,
									transformDraggedElement
								}}
								on:consider={handleDndConsiderItem}
								on:finalize={handleDndFinalizeItem}
							>
								{#if $playlist_ch_items != null && $playlist_ch_items.length > 0}
									{#each $playlist_ch_items as channel, channelIdx (channel.id)}
										<tr id="animate" animate:flip={{ duration: flipDurationMs }}>
											<td>{channel.title}</td>
											<td>{channel.tvg_id}</td>
											<td class="h-4 w-5 items-center hover:bg-red-900">
												<button
													class="btn h-full max-h-4 w-full"
													on:click={() => removeChannelItem(channel.id)}
												>
													<div>
														<Icon icon="icon-park-outline:delete" />
													</div>
												</button>
											</td>
											{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
												<div
													in:fade={{ duration: 200, easing: cubicIn }}
													class="custom-shadow-item"
												>
													{channel.name}
												</div>
											{/if}
										</tr>
									{/each}
								{:else}
									<p class="h-20">No channels found</p>
								{/if}
							</tbody>
						</table>
					</div>
				</div>
			</div>
			{#if !isNew}
				<hr class="!border-t-2" />
				<div class="max-h-80 overflow-y-scroll">
					<table class="table table-hover justify-center text-center shadow-md">
						<thead>
							<tr id="thead">
								<th>Title</th>
								<th>tvg-id</th>
								<th>Score</th>
							</tr>
						</thead>
						{#if $playlistMatches != null && $playlistMatches.length > 0}
							<tbody
								use:dndzone={{
									items: $playlistMatches,
									flipDurationMs,
									type: dndTypeChannels,
									dropFromOthersDisabled
								}}
								on:consider={handleDndConsiderMatch}
								on:finalize={handleDndFinalizeMatch}
							>
								{#each $playlistMatches as channel, channelIdx (channel.id)}
									<tr id="animate" animate:flip={{ duration: flipDurationMs }}>
										<td>{channel.name}</td>
										<td>{channel.tvgid}</td>
										<td>{channel.score}</td>

										{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
											<div
												in:fade={{ duration: 200, easing: cubicIn }}
												class="custom-shadow-item"
											>
												{channel.name}
											</div>
										{/if}
									</tr>
								{/each}
							</tbody>
						{:else}
							<p>No channels found</p>
						{/if}
					</table>
				</div>
			{/if}
			<div data-popup="popupTvgid" class="card max-h-52 w-fit p-4 shadow-xl overflow-y-scroll">
				<Autocomplete
					bind:input={inputTvgid}
					options={tvgidOptions}
					allowlist={tvgidList}
					on:selection={onInputChipSelect}
				/>
				<div class="arrow bg-surface-100-800-token" />
			</div>
		</form>
		<footer class="modal-footer {parent.regionFooter}">
			{#if !isNew}
				<button class="variant-ghost-error btn items-center" on:click={deleteChannel}>
					<Icon icon="icon-park-outline:delete" width="20" height="20" />
					<span>Delete</span>
				</button>
			{/if}
			<button class="btn items-center {parent.buttonNeutral}" on:click={parent.onClose}>
				{parent.buttonTextCancel}</button
			>
			<button class="btn items-center {parent.buttonPositive}" on:click={onFormSubmit}>
				<Icon icon="icon-park-outline:save-one" width="20" height="20" />
				<span>Save Changes</span>
			</button>
		</footer>
	</div>

	<section class="logoList card p-2 shadow-2xl" data-popup="popupLogo">
		<p class="h3 p-1 text-center font-bold">Choose Logo</p>
		<div
			class="h-fit max-h-96 w-fit overflow-y-scroll rounded-lg border-transparent bg-cover p-2 shadow ring-4 ring-blue-500/50"
		>
			{#if $logos != null && $logos.length > 0}
				<section class="grid grid-cols-7 items-center justify-items-center space-x-4 space-y-1">
					{#each $logos as logo, logoIdx (logo.id)}
						<button
							id="chooseImage"
							class="btn h-auto w-20 items-center p-1"
							on:click={() => chooseImage(logo)}
						>
							<img src={logo.image} alt="" />
						</button>
					{/each}
				</section>
			{/if}
		</div>
		<FileButton
			class="mt-2 text-center"
			name="files"
			bind:files
			accept=".png,.jpg,.webp,.svg"
			on:change={onUploadHandler}
			>Upload New Image
		</FileButton>
	</section>
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
	#thead {
		position: relative;
		text-align: center;
		height: 0.25rem;
		max-height: 0.25rem;
	}
	.center {
		text-align: center;
		justify-content: center;
		align-items: center;
		width: 100%;
	}
</style>
