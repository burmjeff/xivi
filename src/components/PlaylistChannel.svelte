<!-- PlaylistChannel.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { playlists } from '@xivi/stores/playlist_store';
	import type { PlaylistChannel } from '@xivi/data/playlist_entities';

	export let playlistIdx: number;
	export let groupId: string;
	export let groupIdx: number;

	const updatePlaylistChannels = async () => {
		const response = await fetch(`/api/playlist/group/${groupId}/channels`);
		const data = await response.json();
		return data.playlistchannels;
	};

	onMount(async () => {
		$playlists[playlistIdx].playlistGroups[groupIdx].playlistChannels = [];
		const fetchedData = await updatePlaylistChannels();
        fetchedData.forEach(function (channel: PlaylistChannel) {
			$playlists[playlistIdx].playlistGroups[groupIdx].playlistChannels.push(channel);
            $playlists[playlistIdx].playlistGroups[groupIdx].playlistChannels = $playlists[playlistIdx].playlistGroups[groupIdx].playlistChannels
		});
	});
</script>


{#if $playlists[playlistIdx].playlistGroups[groupIdx].playlistChannels != null && $playlists[playlistIdx].playlistGroups[groupIdx].playlistChannels.length > 0}
	<table class="playlistChannel table">
		<thead>
			<tr>
				<th>Logo</th>
				<th>Name</th>
				<th>tvg-id</th>
			</tr>
		</thead>
		<tbody>
			{#each $playlists[playlistIdx].playlistGroups[groupIdx].playlistChannels as channel, i}
				<tr>
					<td><img class="w-14" src={channel.tvg_logo} alt="Logo" /></td>
					<td>{channel.name}</td>
					<td>{channel.tvgid}</td>
				</tr>
			{/each}
		</tbody>
	</table>
{:else}
    <p>No playlist channels found</p>
{/if}