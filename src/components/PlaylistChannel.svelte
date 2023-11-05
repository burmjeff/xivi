<!-- PlaylistChannel.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { playlistChannels } from '@xivi/stores/playlist_store';

	export let playlistId: number;
	export let groupId: number;

	let channels: typeof playlistChannels;

	onMount(async () => {
		fetch(`/api/playlist/${playlistId}/group/${groupId}/channels`)
			.then((response) => response.json())
			.then((data) => {
				console.log(data);
				playlistChannels.set(data.playlistchannels);
			})
			.catch((error) => {
				console.log(error);
				return [];
			});
	});
</script>

<table class="playlistChannel table">
	<thead>
		<tr>
			<th>Logo</th>
			<th>Name</th>
			<th>tvg-id</th>
		</tr>
	</thead>
	<tbody>
		{#each $playlistChannels as channel, i}
			<tr>
				<td><img class="w-14" src={channel.tvg_logo} alt="Logo" /></td>
				<td>{channel.name}</td>
				<td>{channel.tvgid}</td>
			</tr>
		{/each}
	</tbody>
</table>
