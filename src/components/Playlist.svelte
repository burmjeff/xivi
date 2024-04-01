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

	let playlistSettings: PopupSettings = {
		// Set the event as: click | hover | hover-click
		event: 'click',
		// Provide a matching 'data-popup' value.
		target: 'addPlaylistPopup'
	};

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

	async function addPlaylist() {
		const inputName = (document.querySelector('.playlist_name input') as HTMLInputElement).value;
		const inputUrl = (document.querySelector('.playlist_url input') as HTMLInputElement).value;
		if (inputName !== '' && inputUrl !== '') {
			const newPlaylist = {
				name: inputName,
				url: inputUrl
			};
			window.console.log('PLAYLIST: ', newPlaylist);

			try {
				const response = await fetch('/api/playlist', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newPlaylist)
				});
				const data = await response.json();
				console.log('Created playlist:', data);
				$playlists.push(data.playlist);
				$playlists = [...$playlists];
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
		<button class="btn btn-md" use:popup={playlistSettings} use:popup={addPlTooltip}>
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

<div class="card gap-4 p-4" data-popup="addPlaylistPopup">
	<header class="justify-center text-center text-2xl font-bold">Add Playlist</header>
	<div class="space-y-4">
		<label class="playlist_name">
			<span>Playlist Name</span>
			<input class="input" type="text" placeholder="Playlist Name" />
		</label>
		<label class="playlist_url">
			<span>Playlist url</span>
			<input class="input" type="url" placeholder="https://example.com/xivi.m3u" />
		</label>
		<label class="submit_button">
			<button class="btn bg-primary-500" on:click={addPlaylist}>Add Playlist</button>
		</label>
	</div>
</div>

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
