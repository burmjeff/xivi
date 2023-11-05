<!-- Playlist.svelte -->

<script lang="ts">
    import PlaylistGroup from './PlaylistGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, type PopupSettings } from '@skeletonlabs/skeleton';
    import { playlists } from '@xivi/stores/playlist_store';
    import {flip} from 'svelte/animate';
    import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
    import type { Playlist } from '@xivi/data/playlist_entities';
    import {fade} from 'svelte/transition';
    import {cubicIn} from 'svelte/easing';

    let shouldIgnoreDndEvents = false;

    const updatePlaylists = async () => {
        const response = await fetch('/api/playlists');
        const data = await response.json();
        return data.playlists;
    }
  
    onMount(async () => {
        const fetchedData = await updatePlaylists();
        playlists.set(fetchedData);
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

    const flipDurationMs = 300;
    function handleDndConsider(e: CustomEvent<DndEvent<Playlist>>) {
        console.warn(`got consider ${JSON.stringify(e.detail, null, 2)}`);
        const {trigger, id} = e.detail.info;
        if (trigger === TRIGGERS.DRAG_STARTED) {
            console.warn(`copying ${id}`);
            const idx = $playlists.findIndex(item => item.id === Number(id));
            const newId = `${id}_copy_${Math.round(Math.random()*100000)}`;
						// the line below was added in order to be compatible with version svelte-dnd-action 0.7.4 and above 
					  e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
            e.detail.items.splice(idx, 0, {...$playlists[idx], id: Number(newId)});
            $playlists = e.detail.items;
            shouldIgnoreDndEvents = true;
        }
        else if (!shouldIgnoreDndEvents) {
            $playlists = e.detail.items;
        }
        else {
            $playlists = [...$playlists];
        }
    }
    function handleDndFinalize(e: CustomEvent<DndEvent<Playlist>>) {
        console.warn(`got finalize ${JSON.stringify(e.detail, null, 2)}`);
        if (!shouldIgnoreDndEvents) {
            $playlists = e.detail.items;
        }
        else {
            $playlists = [...$playlists];
            shouldIgnoreDndEvents = false;
        }
    }
</script>

<section class="playlists card card-hover p-1">
    <header class="playlists-header flex justify-center items-center space-x-4">
        <h3 class="h3 font-bold">Playlists</h3>
        <button class="btn btn-sm variant-ringed-primary" use:popup={playlistSettings}>+ add new</button>
    </header>
    <div class="playlists-viewport flex-none min-w-full overflow-hidden lg:overflow-auto max-h-[42rem]">
        {#if $playlists.length > 0}
            <Accordion>
                <section use:dndzone={{items: $playlists, flipDurationMs}} on:consider={handleDndConsider} on:finalize={handleDndFinalize}>
                    {#each $playlists as playlist(playlist.id)}
                        <div id="div1" animate:flip={{duration: flipDurationMs}}>
                            <AccordionItem key={playlist.id}>
                                <svelte:fragment slot="summary"><h4>{playlist.name}</h4></svelte:fragment>
                                <svelte:fragment slot="content">
                                    <PlaylistGroup playlistId={playlist.id} />
                                </svelte:fragment>
                            </AccordionItem>
                            {#if playlist[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
                                <div in:fade={{duration:200, easing: cubicIn}} class='custom-shadow-item'>{playlist.name}</div>
                            {/if}
                        </div>
                    {/each}
                </section>
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

<style>
    #div1 {
		position: relative;
		text-align: center;
		margin: 0.2em;
		padding: 0.3em;
	}
    .custom-shadow-item {
		position: absolute;
		top: 0; left:0; right: 0; bottom: 0;
		visibility: visible;
		border: 3px dashed grey;
		background: lightblue;
		opacity: 0.6;
		margin: 0;
	}
</style>