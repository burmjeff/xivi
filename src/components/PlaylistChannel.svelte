<!-- ChannelTable.svelte -->
<script lang="ts">
    import { onMount } from 'svelte';
    import { playlistChannels } from '../stores/playliststore';
  
    export let playlistId: number;
    export let groupId: number;
  
    let channels: typeof playlistChannels

    onMount(async () => {
        fetch('/api/playlists')
        .then(response => response.json())
        .then(data => {
            console.log(data);
            playlistChannels.set(data.playlistchannels);
        }).catch(error => {
            console.log(error);
            return [];
        });
        $channels = $playlistChannels.filter((channel: { group_id: number }) => channel.group_id === groupId);
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
      {#each $channels as channel}
        <tr>
          <td>{channel.name}</td>
          <td>{channel.tvgid}</td>
          <td>{channel.group_id}</td>
        </tr>
      {/each}
    </tbody>
  </table>
  