<!-- Playlist.svelte -->
<script lang="ts">
    import PlaylistGroup from './PlaylistGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
    import { playlists } from '../stores/playlist_store';

    onMount(async () => {
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
</script>

<div class="card card-hover p-4">Playlists
    {#if $playlists.length > 0}
        <Accordion>
            {#each $playlists as playlist}
                <AccordionItem key={playlist.id}>
                    <svelte:fragment slot="lead">{playlist.id}</svelte:fragment>
                    <svelte:fragment slot="summary"><h3>{playlist.name}</h3></svelte:fragment>
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