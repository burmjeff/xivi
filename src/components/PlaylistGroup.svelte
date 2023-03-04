<!-- PlaylistGroup.svelte -->
<script lang="ts">
    import PlaylistChannel from './PlaylistChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
    import {playlistGroups} from '@xivi/stores/playlist_store';
  
    export let playlistId: number;
  
    onMount(async () => {
        fetch(`/api/playlist/${playlistId}/groups`)
        .then(response => response.json())
        .then(data => {
            console.log(data);
            playlistGroups.set(data.playlistgroups);
        }).catch(error => {
            console.log(error);
            return [];
        });
        });
  </script>

  <Accordion>
    {#each $playlistGroups as group}
        <AccordionItem>
            <svelte:fragment slot="lead">{group.id}</svelte:fragment>
            <svelte:fragment slot="summary"><h3>{group.name}</h3></svelte:fragment>
            <svelte:fragment slot="content">
              <PlaylistChannel playlistId={playlistId} groupId={group.id} />
            </svelte:fragment>
        </AccordionItem>
    {/each}
</Accordion>