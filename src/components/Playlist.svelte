<!-- Playlist.svelte -->

<script lang="ts">
	import PlaylistGroup from './PlaylistGroup.svelte';
	import { onMount } from 'svelte';
	import { Accordion, Modal } from '@skeletonlabs/skeleton-svelte';
	import {
		FloatingArrow,
		arrow,
		autoUpdate,
		flip,
		offset,
		useDismiss,
		useFloating,
		useHover,
		useInteractions,
		useRole
	} from '@skeletonlabs/floating-ui-svelte';
	import PlaylistSettings from './modals/PlaylistSettings.svelte';
	import { playlists } from '@xivi/stores/playlist_store';
	import type { Playlist } from '@xivi/data/playlist_entities';
	import Icon from '@iconify/svelte';
	import { fade } from 'svelte/transition';

	let playlistModalOpen = $state(false);
	let deleteModalOpen = $state(false);
	let currentPlaylistId = $state(0);
	let currentPlaylistName = $state('');
	let currentPlaylistUrl = $state('');
	let isNewPlaylist = $state(false);
	let refreshingPlaylistId = $state<number | null>(null);

	const updatePlaylists = async () => {
		const response = await fetch('/api/playlists');
		const data = await response.json();
		return data.playlists;
	};

	onMount(async () => {
		playlists.set(await updatePlaylists());
	});

	// Floating UI state
	let addPlTooltipOpen = $state(false);
	let refreshTooltipOpen = $state<{ [key: number]: boolean }>({});
	let editTooltipOpen = $state<{ [key: number]: boolean }>({});
	let deleteTooltipOpen = $state<{ [key: number]: boolean }>({});
	let addPlElemArrow: HTMLElement | null = $state(null);
	let accordionItem = $state<string[]>([]);

	// Floating UI setup for add playlist tooltip
	const addPlTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return addPlTooltipOpen;
		},
		onOpenChange: (v) => {
			addPlTooltipOpen = v;
		},
		placement: 'top',
		get middleware() {
			return [offset(10), flip(), addPlElemArrow && arrow({ element: addPlElemArrow })];
		}
	});

	// Interactions for add playlist tooltip
	const addPlTooltipRole = useRole(addPlTooltipFloating.context, { role: 'tooltip' });
	const addPlTooltipHover = useHover(addPlTooltipFloating.context, { move: false });
	const addPlTooltipDismiss = useDismiss(addPlTooltipFloating.context);
	const addPlTooltipInteractions = useInteractions([
		addPlTooltipRole,
		addPlTooltipHover,
		addPlTooltipDismiss
	]);

	// Helper function to create floating UI for playlist buttons with unique arrow elements
	function createTooltipFloating(playlistId: number, tooltipType: 'edit' | 'refresh' | 'delete') {
		const tooltipState = tooltipType === 'edit' ? editTooltipOpen :
							 tooltipType === 'refresh' ? refreshTooltipOpen : deleteTooltipOpen;

		return useFloating({
			whileElementsMounted: autoUpdate,
			get open() {
				return tooltipState[playlistId] || false;
			},
			onOpenChange: (v) => {
				tooltipState[playlistId] = v;
			},
			placement: 'top',
			get middleware() {
				return [offset(10), flip()];
			}
		});
	}

	// Helper function to create interactions for playlist tooltips
	function createTooltipInteractions(floating: any) {
		const role = useRole(floating.context, { role: 'tooltip' });
		const hover = useHover(floating.context, { move: false });
		const dismiss = useDismiss(floating.context);
		return useInteractions([role, hover, dismiss]);
	}



	function getDate(dateStr: string) {
		const date = new Date(dateStr);
		let year = date.getFullYear();
		let month = String(date.getMonth() + 1).padStart(2, '0');
		let day = String(date.getDate()).padStart(2, '0');
		let hours = String(date.getHours()).padStart(2, '0');
		let minutes = String(date.getMinutes()).padStart(2, '0');
		return `${year}-${month}-${day} ${hours}:${minutes}`;
	}

	function modalPlaylist(isNew: boolean, id: number, name: string, url: string) {
		isNewPlaylist = isNew;
		currentPlaylistId = id;
		currentPlaylistName = name;
		currentPlaylistUrl = url;
		playlistModalOpen = true;
	}

	function handlePlaylistClose(formData: any = null) {
		console.log('handlePlaylistClose called with formData:', formData);
		if (formData) {
			addPlaylist(formData, isNewPlaylist, currentPlaylistId);
		}
		playlistModalOpen = false;
	}

	async function addPlaylist(formData: any, isNew: boolean, id: number) {
		let method: string;
		if (formData.name && formData.url) {
			let newPlaylist = {
				id: id,
				name: formData.name,
				url: formData.url
			};

			if (isNew) {
				method = 'POST';
			} else {
				method = 'PUT';
			}

			try {
				const response = await fetch('/api/playlist', {
					method: method,
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newPlaylist)
				});
				if (response.ok) {
					if (isNew) {
						const data = await response.json();
						console.log('Created playlist:', data);
						$playlists.push(data.playlist);
						$playlists = [...$playlists];
					} else {
						console.log('Updated playlist: ', newPlaylist.name);
						$playlists = $playlists.map((playlist) => {
							if (playlist.id === id) {
								return {
									...playlist,
									name: formData.name,
									url: formData.url
								};
							}
							return playlist;
						});
						$playlists = [...$playlists];
					}
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error creating playlist:', error);
			}
		}
	}

	function deletePrompt(playlist: Playlist): void {
		playlist.itemOpen = false;
		currentPlaylistId = playlist.id;
		deleteModalOpen = true;
	}

	function handleDeleteClose(confirm: boolean) {
		if (confirm) {
			deletePlaylist(currentPlaylistId);
		}
		deleteModalOpen = false;
	}

	async function deletePlaylist(playlistId: number) {
		try {
			const response = await fetch(`/api/playlist/${playlistId}`, {
				method: 'DELETE'
			});
			const status = response.status;
			console.log('Deleted playlist, status:', status);
			$playlists = $playlists.filter((t) => t.id != playlistId);
		} catch (error) {
			console.log('Error deleting playlist:', error);
			return;
		}
	}

	async function refreshPlaylist(playlistId: number) {
		try {
			// Set the refreshing state for this playlist
			refreshingPlaylistId = playlistId;

			// Call the API to refresh the playlist
			const response = await fetch(`/api/playlist/${playlistId}/refresh`, {
				method: 'POST'
			});

			if (response.ok) {
				console.log('Refreshing playlist started');

				// Wait a bit to allow the backend to process, then update the UI
				setTimeout(async () => {
					// Fetch updated playlists
					playlists.set(await updatePlaylists());
					// Clear the refreshing state
					refreshingPlaylistId = null;
				}, 2000);
			} else {
				console.error('Error refreshing playlist:', response.status, response.statusText);
				refreshingPlaylistId = null;
			}
		} catch (error) {
			console.log('Error refreshing playlist:', error);
			refreshingPlaylistId = null;
		}
	}
</script>

<section class="playlists h-full w-full p-1">
	<header
		class="playlists-header border-surface-700/30 flex items-center justify-center border-b p-1"
	>
		<h4 class="h4 text-primary-400 font-bold">Playlists</h4>
		<button
			class="btn btn-md self-start"
			onclick={() => modalPlaylist(true, 0, '', '')}
			bind:this={addPlTooltipFloating.elements.reference}
			{...addPlTooltipInteractions.getReferenceProps()}
		>
			<Icon icon="icon-park-twotone:add-one" color="#0a7e85" width="25" height="25" />
			{#if addPlTooltipOpen}
				<div
					bind:this={addPlTooltipFloating.elements.floating}
					style={addPlTooltipFloating.floatingStyles}
					{...addPlTooltipInteractions.getFloatingProps()}
					class="floating glass card p-2 shadow-lg"
					transition:fade={{ duration: 200 }}
				>
					<p class="text-sm font-medium"><strong>Add a new Playlist</strong></p>
					<FloatingArrow
						bind:ref={addPlElemArrow}
						context={addPlTooltipFloating.context}
						fill="#1e293b"
					/>
				</div>
			{/if}
		</button>
	</header>
	<Accordion value={accordionItem} onValueChange={(e) => (accordionItem = e.value)} collapsible>
		<div id="accord" class="playlists-viewport min-w-full overflow-auto">
			{#if $playlists != null && $playlists.length > 0}
				{#each $playlists as playlist, index (playlist.id)}
					{@const editFloating = createTooltipFloating(playlist.id, 'edit')}
					{@const editInteractions = createTooltipInteractions(editFloating)}
					{@const refreshFloating = createTooltipFloating(playlist.id, 'refresh')}
					{@const refreshInteractions = createTooltipInteractions(refreshFloating)}
					{@const deleteFloating = createTooltipFloating(playlist.id, 'delete')}
					{@const deleteInteractions = createTooltipInteractions(deleteFloating)}
					<div class="card mb-1 shadow-md">
						<Accordion.Item value={playlist.name}>
							{#snippet control()}
								<div class="flex w-full cursor-pointer flex-row items-center">
									<h4 class="flex-grow text-lg">{playlist.name}</h4>
									<div class="flex flex-row items-center gap-1">
										<span class="p-1 text-xs text-green-600"
											>Updated at: {getDate(playlist.updated_at)}</span
										>

										<button
											class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
											onclick={(e) => {
												e.stopPropagation();
												modalPlaylist(false, playlist.id, playlist.name, playlist.url);
											}}
											bind:this={editFloating.elements.reference}
											{...editInteractions.getReferenceProps()}
											><Icon icon="icon-park-outline:edit-two" width="18" height="18" />
											{#if editTooltipOpen[playlist.id]}
												<div
													bind:this={editFloating.elements.floating}
													style={editFloating.floatingStyles}
													{...editInteractions.getFloatingProps()}
													class="floating glass card p-2 shadow-lg"
													transition:fade={{ duration: 200 }}
												>
													<p class="text-sm font-medium"><strong>Edit Playlist</strong></p>
												</div>
											{/if}
										</button>
										<button
											class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
											onclick={(e) => {
												e.stopPropagation();
												refreshPlaylist(playlist.id);
											}}
											disabled={refreshingPlaylistId === playlist.id}
											class:animate-spin={refreshingPlaylistId === playlist.id}
											bind:this={refreshFloating.elements.reference}
											{...refreshInteractions.getReferenceProps()}
											><Icon icon="icon-park-outline:refresh-one" width="18" height="18" />
											{#if refreshTooltipOpen[playlist.id]}
												<div
													bind:this={refreshFloating.elements.floating}
													style={refreshFloating.floatingStyles}
													{...refreshInteractions.getFloatingProps()}
													class="floating glass card p-2 shadow-lg"
													transition:fade={{ duration: 200 }}
												>
													<p class="text-sm font-medium"><strong>Refresh Playlist</strong></p>
												</div>
											{/if}
										</button>
										<button
											class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
											onclick={(e) => {
												e.stopPropagation();
												deletePrompt(playlist);
											}}
											bind:this={deleteFloating.elements.reference}
											{...deleteInteractions.getReferenceProps()}
											><Icon icon="icon-park-outline:delete" width="18" height="18" />
											{#if deleteTooltipOpen[playlist.id]}
												<div
													bind:this={deleteFloating.elements.floating}
													style={deleteFloating.floatingStyles}
													{...deleteInteractions.getFloatingProps()}
													class="floating glass card p-2 shadow-lg"
													transition:fade={{ duration: 200 }}
												>
													<p class="text-sm font-medium"><strong>Delete Playlist</strong></p>
												</div>
											{/if}
										</button>
									</div>
								</div>
							{/snippet}
							{#snippet panel()}
								<PlaylistGroup playlistId={playlist.id} playlistIdx={index} />
							{/snippet}
						</Accordion.Item>
					</div>
				{/each}
			{:else}
				<div class="flex flex-col items-center justify-center px-2 py-2 text-center">
					<Icon icon="mdi:folder-off" class="text-surface-500 mb-3" width="48" height="48" />
					<h3 class="text-surface-300 mb-2 text-xl font-medium">No Playlists Found</h3>
					<p class="text-surface-400 max-w-md">
						Add your first playlist by clicking the "Add Playlist" button above.
					</p>
				</div>
			{/if}
		</div>
	</Accordion>
</section>

<Modal
	open={playlistModalOpen}
	onOpenChange={(e) => (playlistModalOpen = e.open)}
	contentBase="card bg-surface-100-900 shadow-xl"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
		<PlaylistSettings
			parent={{ onClose: handlePlaylistClose }}
			isNew={isNewPlaylist}
			id={currentPlaylistId}
			name={currentPlaylistName}
			url={currentPlaylistUrl}
		/>
	{/snippet}
</Modal>

<Modal
	open={deleteModalOpen}
	onOpenChange={(e) => (deleteModalOpen = e.open)}
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
		<header class="text-2xl font-bold">Please Confirm</header>
		<article>Are you sure you wish to delete this playlist?</article>
		<footer class="flex justify-end gap-4">
			<button class="btn preset-outlined-surface-500" onclick={() => handleDeleteClose(false)}
				>Cancel</button
			>
			<button class="btn preset-tonal-error" onclick={() => handleDeleteClose(true)}>Delete</button>
		</footer>
	{/snippet}
</Modal>

<style>
	#accord {
		max-height: 72vh;
		height: 72vh;
	}
</style>
