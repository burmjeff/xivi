<!-- Playlist.svelte -->

<script lang="ts">
	import PlaylistGroup from './PlaylistGroup.svelte';
	import { onMount } from 'svelte';
	import {
		Accordion,
		AccordionItem,
		popup,
		getModalStore,
		type ModalSettings,
		type PopupSettings
	} from '@skeletonlabs/skeleton';
	import { playlists } from '@xivi/stores/playlist_store';
	import Icon from '@iconify/svelte';

	const modalStore = getModalStore();

	const updatePlaylists = async () => {
		const response = await fetch('/api/playlists');
		const data = await response.json();
		return data.playlists;
	};

	onMount(async () => {
		playlists.set(await updatePlaylists());
	});

	const addPlTooltip: PopupSettings = {
		event: 'hover',
		target: 'addPlTooltip',
		placement: 'top'
	};

	function getDate(dateStr: string) {
		const date = new Date(dateStr);
		let year = date.getFullYear();
		let month = String(date.getMonth() + 1).padStart(2, '0');
		let day = String(date.getDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
	}

	function modalPlaylist(isNew: boolean, id: number, name: string, url: string) {
		new Promise<boolean>((resolve) => {
			const modal: ModalSettings = {
				type: 'component',
				component: 'modalPlaylistSettings',
				meta: {
					isNew: isNew,
					id: id,
					name: name,
					url: url
				},
				response: (r: boolean) => {
					resolve(r);
				}
			};
			modalStore.trigger(modal);
		}).then((r: any) => {
			if (r) {
				addPlaylist(r, isNew, id);
			}
		});
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

	function deletePrompt(playlistId: number): void {
		const modal: ModalSettings = {
			type: 'confirm',
			title: 'Please Confirm',
			body: 'Are you sure you wish to delete this playlist?',
			// TRUE if confirm pressed, FALSE if cancel pressed
			response: (r: boolean) => {
				if (r) deletePlaylist(playlistId);
			}
		};
		modalStore.trigger(modal);
	}

	async function deletePlaylist(playlistId: number) {
		try {
			const response = await fetch(`/api/playlist/${playlistId}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted playlist:', data);
			$playlists = $playlists.filter((t) => t.id != playlistId);
			modalStore.close();
		} catch (error) {
			console.log('Error deleting playlist:', error);
			return;
		}
	}
</script>

<section class="playlists card card-hover p-1">
	<header class="playlists-header flex items-center justify-center">
		<h3 class="h3 font-bold">Playlists</h3>
		<button
			class="btn btn-md"
			on:click={() => modalPlaylist(true, 0, '', '')}
			use:popup={addPlTooltip}
		>
			<Icon icon="icon-park-twotone:add-one" color="#0a7e85" width="25" height="25" />
		</button>
	</header>
	<Accordion>
		<div id="accord" class="playlists-viewport min-w-full overflow-auto">
			{#if $playlists != null && $playlists.length > 0}
				{#each $playlists as playlist, index (playlist.id)}
					<AccordionItem class="card shadow-md mb-1" key={playlist.id} bind:open={playlist.itemOpen}>
						<svelte:fragment slot="summary">
							<div class="flex flex-row items-center">
								<h4 class="text-lg">{playlist.name}</h4>
								<span class="text-green-600 text-xs ml-auto p-1">Updated at: {(getDate(playlist.updated_at))}</span>
								<button
									class="btn btn-md"
									on:click={() => modalPlaylist(false, playlist.id, playlist.name, playlist.url)}
									use:popup={addPlTooltip}
								>
									<Icon icon="icon-park-outline:edit-two" width="18" height="18" />
								</button>
								<button
									class="btn-icon btn-icon-sm inset-y-0 !bg-transparent"
									on:click={() => {
										(playlist.itemOpen = true), deletePrompt(playlist.id);
									}}
									><Icon icon="icon-park-outline:delete" width="18" height="18" />
								</button>
							</div>
						</svelte:fragment>
						<svelte:fragment slot="content">
							<PlaylistGroup playlistId={playlist.id} playlistIdx={index} />
						</svelte:fragment>
					</AccordionItem>
				{/each}
			{:else}
				<p>No playlists found</p>
			{/if}
		</div>
	</Accordion>
</section>

<div class="card variant-filled-secondary p-2" data-popup="addPlTooltip">
	<p>Add New Playlist</p>
	<div class="variant-filled-secondary arrow" />
</div>

<style>
	#accord {
		max-height: 76vh;
		height: 76vh;
	}
</style>
