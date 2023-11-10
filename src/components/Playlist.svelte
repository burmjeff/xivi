<!-- Playlist.svelte -->

<script lang="ts">
    import PlaylistGroup from './PlaylistGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, getModalStore, type ModalSettings, type PopupSettings } from '@skeletonlabs/skeleton';
    import { playlists } from '@xivi/stores/playlist_store';
    import IconParkOutlineDelete from '~icons/icon-park-outline/delete';

    const modalStore = getModalStore();

    const updatePlaylists = async () => {
        const response = await fetch('/api/playlists');
        const data = await response.json();
        return data.playlists;
    }
  
    onMount(async () => {
        playlists.set(await updatePlaylists());
    });

    let playlistSettings: PopupSettings = {
        // Set the event as: click | hover | hover-click
        event: 'click',
        // Provide a matching 'data-popup' value.
        target: 'addPlaylistPopup'
    };

    async function addPlaylist() {
        const inputName = (document.querySelector('.playlist_name input') as HTMLInputElement).value;
        const inputUrl = (document.querySelector('.playlist_url input') as HTMLInputElement).value;
        if (inputName !=='' &&  inputUrl !=='') {
            const newPlaylist = {
                name: inputName,
                url: inputUrl
            };
            window.console.log('PLAYLIST: ', newPlaylist)

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
                $playlists.push(data.playlists);
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
			},
		};
		modalStore.trigger(modal);
	}
    
    async function deletePlaylist(playlistId: number) {
		try {
			const response = await fetch(`/api/playlist/${playlistId}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted template playlist:', data);
			$playlists = $playlists.filter(t => t.id != playlistId)
			modalStore.close();
		} catch (error) {
			console.log('Error deleting template playlist:', error);
			return;
		}
	}

</script>

<section class="playlists card card-hover p-1">
    <header class="playlists-header flex justify-center items-center space-x-4">
        <h3 class="h3 font-bold">Playlists</h3>
        <button class="btn btn-sm variant-ringed-primary" use:popup={playlistSettings}>+ add new</button>
    </header>
    <div class="playlists-viewport flex-none min-w-full overflow-auto max-h-[42rem]">
        {#if $playlists.length > 0}
            <Accordion>
                    {#each $playlists as playlist, index (playlist.id)}
                            <AccordionItem key={playlist.id} bind:open={playlist.itemOpen}>
                                <svelte:fragment slot="summary">
                                    <div class="flex flex-row">
                                        <h4>{playlist.name}</h4>
                                        <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" on:click={() => {playlist.itemOpen = true, deletePrompt(playlist.id)}}><i><IconParkOutlineDelete/></i></button>
                                    </div>
                                </svelte:fragment>
                                <svelte:fragment slot="content">
                                    <PlaylistGroup playlistId={playlist.id} playlistIdx={index} />
                                </svelte:fragment>
                            </AccordionItem>
                    {/each}
            </Accordion>
        {:else}
            <p>No playlists found</p>
        {/if}
    </div>
</section>

<div class="card p-4 gap-4" data-popup="addPlaylistPopup">
	<h2>Add Playlist</h2>
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