<!-- ChannelTable.svelte -->
<script lang="ts">
    import { onMount } from 'svelte';
    import { playlistChannels } from '../stores/playliststore';
  
    export let playlistId: number;
    export let groupId: number;
  
    let channels: typeof playlistChannels

    onMount(async () => {
        fetch(`/api/playlist/${playlistId}/group/${groupId}/channels`)
        .then(response => response.json())
        .then(data => {
            console.log(data);
            playlistChannels.set(data.playlistchannels);
        }).catch(error => {
            console.log(error);
            return [];
        });
        });

  </script>
  
  <table class="table">
    <thead>
      <tr>
        <th>Name</th>
        <th>Description</th>
        <th>Group ID</th>
      </tr>
    </thead>
    <tbody>
      {#each $playlistChannels as channel}
        <tr>
          <td>{channel.group_id}</td>
          <td>{channel.name}</td>
          <td>{channel.tvgid}</td>
        </tr>
      {/each}
    </tbody>
  </table>
  