<!-- ChannelSettings.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import { templateGroups } from '@xivi/stores/template_store';
	import Icon from '@iconify/svelte';
	import { writable } from 'svelte/store';
	import { FileUpload, Combobox } from '@skeletonlabs/skeleton-svelte';
	import xivi from '@xivi/lib/assets/xivi.png';
	import type { Match, PlaylistChannel } from '@xivi/data/playlist_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';
	import { playlistMatches } from '@xivi/stores/playlist_store';
	import { logos } from '@xivi/stores/logo_store';
	import type { Logo } from '@xivi/data/logo_entities';
	import {
		FloatingArrow,
		arrow,
		autoUpdate,
		flip as floatingFlip,
		offset,
		useDismiss,
		useFloating,
		useClick,
		useInteractions,
		useRole
	} from '@skeletonlabs/floating-ui-svelte';

	const { parent, isNew, groupIdx, channelIdx } = $props();

	const playlist_ch_items = writable<PlaylistChannel[]>([]);

	let newImg = false;
	let logoName: string;

	let selectedTvgid = $state(['']);

	interface TvgidOptions {
		label: string;
		value: string;
	}
	let tvgidOptions: TvgidOptions[] = $state([]);

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
	} = $state({
		id: 0,
		name: '',
		tvgid: '',
		logoid: 0,
		logo: xivi
	});

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

	const getTvgids = async () => {
		const response = await fetch(`/api/epg/tvgids`);
		const data = await response.json();
		return data.tvgids;
	};

	// Floating UI state for logo popup
	let logoPopupOpen = $state(false);
	let elemArrow: HTMLElement | null = $state(null);

	onMount(async () => {
		tvgidOptions = [];
		const fetchedTvgids = await getTvgids();
		if (fetchedTvgids !== null && typeof fetchedTvgids !== 'undefined') {
			const tvgidList = fetchedTvgids.map((tvgid: string) => {
				return {
					label: `${tvgid}`,
					value: `${tvgid}`
				};
			});

			if (tvgidList.length > 0) {
				tvgidOptions.push(...tvgidList);
			}
		}
		if (formData.tvgid != '') {
			selectedTvgid[0] = formData.tvgid;
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

	// Floating UI setup for logo popup
	const logoFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return logoPopupOpen;
		},
		onOpenChange: (v) => {
			logoPopupOpen = v;
		},
		placement: 'right',
		get middleware() {
			return [offset(10), floatingFlip(), elemArrow && arrow({ element: elemArrow })];
		}
	});

	// Interactions for logo popup
	const logoRole = useRole(logoFloating.context);
	const logoClick = useClick(logoFloating.context);
	const logoDismiss = useDismiss(logoFloating.context);
	const logoInteractions = useInteractions([logoRole, logoClick, logoDismiss]);

	const uploadImage = async () => {
		const response = await fetch(`/api/logo`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json'
			},
			body: JSON.stringify({
				image: formData.logo,
				name: logoName
			})
		});
		const data = await response.json();
		return data.logo;
	};

	function onUploadHandler(event: any) {
		const reader = new FileReader();
		reader.onload = (event) => {
			const image = event.target!.result;
		};
		reader.readAsDataURL(event.details.acceptedFiles[0]);
		const result = reader.result;
		if (result) {
			const file = event.details.acceptedFiles[0];
			logoName = file.name.replace(/\.[^/.]+$/, '');
			formData.logo = result.toString();
			newImg = true;
		}
	}

	async function chooseImage(logo: Logo) {
		formData.logoid = logo.id;
		formData.logo = logo.image;
	}

	async function onFormSubmit(): Promise<void> {
		console.log('Form submitted with data:', formData);
		if (newImg) {
			const fetchedData = await uploadImage();
			formData.logoid = fetchedData.id;
			formData.logo = fetchedData.image;
		}
		formData.tvgid = selectedTvgid[0];
		console.log('Calling parent.onClose with formData:', formData);
		parent.onClose(formData);
	}

	async function deleteChannel() {
		try {
			const response = await fetch(
				`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}`,
				{
					method: 'DELETE'
				}
			);
			const status = response.status;
			console.log('Deleted template channel, status:', status);
			$templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels.filter(
				(t) => Number(t.id) != formData.id
			);
			parent.onClose();
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
			const status = response.status;
			if (response.ok) {
				console.log('Removed template channel item, status:', status);
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

<div class="modal-channel">
	{#if isNew}
		<header class="mb-2 text-center text-2xl font-bold">Add Channel</header>
	{:else}
		<header class="mb-2 text-center text-2xl font-bold">Channel Settings</header>
	{/if}
	<form
		class="modal-form border-surface-500 rounded-container bg-surface-800/20 space-y-2 border p-1"
	>
		<div class="playlist_ch_items grid grid-cols-5 space-x-4">
			<div class="form col-span-2 px-2">
				<label class="channel_name">
					<span>Channel Name</span>
					<input
						class="input variant-form-material"
						type="text"
						bind:value={formData.name}
						placeholder=""
					/>
				</label>
				<div class="channel_tvgid">
					<span>Channel tvgid</span>
					<Combobox value={selectedTvgid} onValueChange={(e) => (selectedTvgid = e.value)}>
						<div class="relative">
							<Combobox.Input
								class="input variant-form-material w-full"
								placeholder="Select or type..."
							/>
						</div>
						<Combobox.Content class="glass card z-[9999] max-h-48 overflow-y-auto p-2 shadow-lg">
							{#each tvgidOptions as option}
								<Combobox.Item
									value={option.value as any}
									label={option.label}
									class="hover:bg-surface-700/30 flex cursor-pointer justify-between rounded px-2 py-1"
								>
									<span>{option.label}</span>
								</Combobox.Item>
							{/each}
						</Combobox.Content>
					</Combobox>
				</div>
				<div class="channel_logo">
					<div class="grid w-64 grid-cols-2 items-center space-x-10 p-1">
						<img class="h-auto max-h-32 w-auto" src={formData.logo} alt="Logo" />
						<button
							class="fill-primary-500 btn h-fit w-fit"
							onclick={updateLogos}
							bind:this={logoFloating.elements.reference}
							{...logoInteractions.getReferenceProps()}>Choose Logo</button
						>
					</div>
				</div>
			</div>
			<div class="col-span-3 max-h-72 px-2">
				<header class="mb-2 justify-center text-center font-bold">Current Playlist Channels</header>
				<div class="max-h-72 overflow-y-scroll">
					<table class="table justify-center text-center shadow-md">
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
							onconsider={handleDndConsiderItem}
							onfinalize={handleDndFinalizeItem}
						>
							{#if $playlist_ch_items != null && $playlist_ch_items.length > 0}
								{#each $playlist_ch_items as channel, channelIdx (channel.id)}
									<tr id="animate" animate:flip={{ duration: flipDurationMs }}>
										<td>{channel.title}</td>
										<td>{channel.tvg_id}</td>
										<td class="h-4 w-5 items-center hover:bg-red-900">
											<button
												class="btn h-full max-h-4 w-full"
												onclick={() => removeChannelItem(channel.id)}
											>
												<div>
													<Icon icon="icon-park-outline:delete" />
												</div>
											</button>
										</td>
										{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
											<td in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
												{channel.title}
											</td>
										{/if}
									</tr>
								{/each}
							{:else}
								<tr class="h-20"><td>No channels found</td></tr>
							{/if}
						</tbody>
					</table>
				</div>
			</div>
		</div>
		{#if !isNew}
			<hr class="border-t-2!" />
			<div class="max-h-80 overflow-y-scroll">
				<table class="table justify-center text-center shadow-md">
					<thead>
						<tr id="thead">
							<th>Title</th>
							<th>tvg-id</th>
							<th>Score</th>
						</tr>
					</thead>
					<tbody
						use:dndzone={{
							items: $playlistMatches,
							flipDurationMs,
							type: dndTypeChannels,
							dropFromOthersDisabled
						}}
						onconsider={handleDndConsiderMatch}
						onfinalize={handleDndFinalizeMatch}
					>
						{#if $playlistMatches != null && $playlistMatches.length > 0}
							{#each $playlistMatches as channel, channelIdx (channel.id)}
								<tr id="animate" animate:flip={{ duration: flipDurationMs }}>
									<td>{channel.name}</td>
									<td>{channel.tvgid}</td>
									<td>{channel.score}</td>

									{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
										<td in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
											{channel.name}
										</td>
									{/if}
								</tr>
							{/each}
						{:else}
							<tr class="h-20"><td>No channels found</td></tr>
						{/if}
					</tbody>
				</table>
			</div>
		{/if}
	</form>
	<footer class="modal-footer flex justify-between pt-2">
		<div>
			{#if !isNew}
				<button class="btn preset-tonal-error" onclick={deleteChannel}>
					<Icon icon="icon-park-outline:delete" width="20" height="20" />
					<span>Delete</span>
				</button>
			{/if}
		</div>
		<div class="flex gap-4">
			<button class="btn preset-outlined-surface-500" onclick={parent.onClose}> Cancel </button>
			<button class="btn preset-filled-primary-500" onclick={onFormSubmit}> Save </button>
		</div>
	</footer>
</div>

{#if logoPopupOpen}
	<div
		bind:this={logoFloating.elements.floating}
		style={logoFloating.floatingStyles}
		{...logoInteractions.getFloatingProps()}
		class="floating glass logoList card p-2 shadow-2xl"
		transition:fade={{ duration: 200 }}
	>
		<p class="h3 p-1 text-center font-bold">Choose Logo</p>
		<div
			class="h-fit max-h-96 w-fit overflow-y-scroll rounded-lg border-transparent bg-cover p-2 shadow-sm ring-4 ring-blue-500/50"
		>
			{#if $logos != null && $logos.length > 0}
				<section class="grid grid-cols-7 items-center justify-items-center space-y-1 space-x-4">
					{#each $logos as logo, logoIdx (logo.id)}
						<button
							id="chooseImage"
							class="btn h-auto w-20 items-center p-1"
							onclick={() => chooseImage(logo)}
						>
							<img src={logo.image} alt="" />
						</button>
					{/each}
				</section>
			{/if}
		</div>
		<div class="mt-2 text-center">
			<FileUpload name="files" accept="image/*" onFileChange={onUploadHandler}>
				<button class="btn preset-filled">
					<span>Upload New Image</span>
				</button>
			</FileUpload>
		</div>
		<FloatingArrow bind:ref={elemArrow} context={logoFloating.context} fill="#1e293b" />
	</div>
{/if}

<style>
	.modal-channel {
		min-width: 800px;
		max-width: 1200px;
		padding: 0.2rem 0.5rem;
	}

	.modal-form {
		background: var(--color-surface-800/20);
		border: 1px solid var(--color-surface-600/50);
		border-radius: 0.5rem;
	}

	.modal-footer {
		margin-top: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--color-surface-600);
	}

	.modal-footer button {
		font-weight: 500;
		padding: 0.4rem 2rem;
	}

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
		pointer-events: none;
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

	/* Responsive design for smaller screens */
	@media (max-width: 1024px) {
		.modal-channel {
			min-width: 90vw;
			max-width: 95vw;
			padding: 1rem;
		}

		.playlist_ch_items {
			grid-template-columns: 1fr;
			gap: 1rem;
		}

		.form {
			order: 1;
		}

		.max-h-72.col-span-3 {
			order: 2;
		}
	}

	@media (max-width: 640px) {
		.modal-channel {
			min-width: unset;
			padding: 0.75rem;
		}

		.modal-footer {
			flex-direction: column;
			gap: 0.75rem;
		}

		.modal-footer > div {
			width: 100%;
		}

		.modal-footer button {
			width: 100%;
		}
	}
</style>
