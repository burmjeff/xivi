<!-- Playlist.svelte -->

<script lang="ts">
	import PlaylistGroup from './PlaylistGroup.svelte';
	import { onMount } from 'svelte';
	import {
		Accordion,
		Modal
	} from '@skeletonlabs/skeleton-svelte';
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
		useRole,
	} from "@skeletonlabs/floating-ui-svelte";
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
	let elemArrow: HTMLElement | null = $state(null);

	// Floating UI setup for add playlist tooltip
	const addPlTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return addPlTooltipOpen;
		},
		onOpenChange: (v) => {
			addPlTooltipOpen = v;
		},
		placement: "top",
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		},
	});

	// Interactions for add playlist tooltip
	const addPlTooltipRole = useRole(addPlTooltipFloating.context, { role: "tooltip" });
	const addPlTooltipHover = useHover(addPlTooltipFloating.context, { move: false });
	const addPlTooltipDismiss = useDismiss(addPlTooltipFloating.context);
	const addPlTooltipInteractions = useInteractions([addPlTooltipRole, addPlTooltipHover, addPlTooltipDismiss]);

	function getDate(dateStr: string) {
		const date = new Date(dateStr);
		let year = date.getFullYear();
		let month = String(date.getMonth() + 1).padStart(2, '0');
		let day = String(date.getDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
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
			let newGroup = {
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
					body: JSON.stringify(newGroup)
				});
				if (response.ok) {
					if (isNew) {
						const data = await response.json();
						console.log('Created playlist:', data);
						$playlists.push(data.playlist);
						$playlists = [...$playlists];
					} else {
						console.log('Updated playlist: ', newGroup.name);
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
</script>

<Modal
	open={playlistModalOpen}
	onOpenChange={(e) => (playlistModalOpen = e.open)}
	contentBase="card bg-surface-100-900 p-4 shadow-xl max-w-screen-sm"
	positionerBase="fixed inset-0 flex justify-center items-center"
	backdropClasses="backdrop-blur-sm fixed inset-0"
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
	contentBase="card bg-surface-100-900 p-4 shadow-xl max-w-screen-sm"
	positionerBase="fixed inset-0 flex justify-center items-center"
	backdropClasses="backdrop-blur-sm fixed inset-0"
>
	{#snippet content()}
		<header class="text-2xl font-bold">Please Confirm</header>
		<article>Are you sure you wish to delete this playlist?</article>
		<footer class="flex justify-end gap-4">
			<button class="btn preset-outlined-surface-500" onclick={() => handleDeleteClose(false)}>Cancel</button>
			<button class="btn preset-tonal-error" onclick={() => handleDeleteClose(true)}>Delete</button>
		</footer>
	{/snippet}
</Modal>

<section class="playlists card card-hover p-1">
	<header class="playlists-header flex items-center justify-center">
		<h3 class="h3 font-bold">Playlists</h3>
		<button
			class="btn btn-md"
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
					class="floating popover-neutral card p-2"
					transition:fade={{ duration: 200 }}
				>
					<p><strong>Add New Playlist</strong></p>
					<FloatingArrow bind:ref={elemArrow} context={addPlTooltipFloating.context} fill="#575969" />
				</div>
			{/if}
		</button>
	</header>
	<Accordion collapsible>
		<div id="accord" class="playlists-viewport min-w-full overflow-auto">
			{#if $playlists != null && $playlists.length > 0}
				{#each $playlists as playlist, index (playlist.id)}
					<div class="card shadow-md mb-1">
						<Accordion.Item value={playlist.name}>
							{#snippet control()}
									<div class="flex flex-row items-center w-full">
										<h4 class="text-lg flex-grow">{playlist.name}</h4>
										<div class="flex flex-row items-center gap-2">
											<span class="text-green-600 text-xs p-1">Updated at: {(getDate(playlist.updated_at))}</span>
											<button
												class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
												onclick={(e) => {
													e.stopPropagation();
													modalPlaylist(false, playlist.id, playlist.name, playlist.url);
												}}
												><Icon icon="icon-park-outline:edit-two" width="18" height="18" />
											</button>
											<button
												class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
												onclick={(e) => {
													e.stopPropagation();
													deletePrompt(playlist);
												}}
												><Icon icon="icon-park-outline:delete" width="18" height="18" />
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
				<p>No playlists found</p>
			{/if}
		</div>
	</Accordion>
</section>



<style>
	#accord {
		max-height: 76vh;
		height: 76vh;
	}
</style>
