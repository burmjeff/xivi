<!-- Playlist.svelte -->

<script lang="ts">
    import PlaylistGroup from './PlaylistGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, type PopupSettings } from '@skeletonlabs/skeleton';
    import { playlists } from '@xivi/stores/playlist_store';

    onMount(
        async () => {
            fetch('/api/playlists')
            .then(response => response.json())
            .then(data => {
                console.log('Fetched data:', data);
                playlists.set(data.playlists);
            }).catch(error => {
                console.log('Error fetching data:', error);
                return [];
            });
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
                playlists.set(data.playlists);
            } catch (error) {
                console.log('Error creating playlist:', error);
                return [];
            }
        }
    }
</script>

<div class="card card-hover p-2">
    <section class="flex items-center space-x-4">
        <h1>Playlists</h1>
        <button class="btn btn-sm variant-ringed-primary" use:popup={playlistSettings}>+ add new</button>
    </section>
    
    {#if $playlists.length > 0}
        <Accordion>
            {#each $playlists as playlist}
                <AccordionItem key={playlist.id}>
                    <svelte:fragment slot="lead">{playlist.id}</svelte:fragment>
                    <svelte:fragment slot="summary"><h4>{playlist.name}</h4></svelte:fragment>
                    <svelte:fragment slot="content">
                        <PlaylistGroup playlistId={playlist.id} />
                    </svelte:fragment>
                </AccordionItem>
            {/each}
        </Accordion>
    {:else}
        <p>No playlists found</p>
    {/if}
</div>
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