<!-- PlaylistSettings.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import {
		popup,
		getModalStore
	} from '@skeletonlabs/skeleton';

	const modalStore = getModalStore();
	let isNew = $modalStore[0].meta.isNew;
	let inputName: string;
	let inputUrl: string;

	let formData: {
		name: string;
		url: string;
	};

	if (!isNew) {
		formData = {
			name: $modalStore[0].meta.name,
			url: $modalStore[0].meta.url
		};
	} else {
		formData = {
			name: '',
			url: ''
		};
	}

	async function onFormSubmit(): Promise<void> {
		if ($modalStore[0].response) $modalStore[0].response(formData);
		modalStore.close();
	}
</script>

{#if $modalStore[0]}
	<div class="modal-playlist max-w-screen card max-h-screen w-fit space-y-4 p-4 shadow-xl">
		{#if isNew}
			<header class="justify-center text-center text-2xl font-bold">Add Playlist</header>
		{:else}
			<header class="justify-center text-center text-2xl font-bold">Modify Playlist</header>
		{/if}
		<div class="space-y-4">
			<label class="playlist_name p-2">
				<span>Playlist Name</span>
				<input
					class="input"
					type="text"
					bind:value={formData.name}
					placeholder="Playlist Name"
				/>
			</label>
			<label class="playlist_url p-2">
				<span>Playlist Url</span>
				<input
					class="input"
					type="text"
					bind:value={formData.url}
					placeholder="https://example.com/xivi.m3u"
				/>
			</label>
			<div class="submit_button justify-center p-2 text-center">
				<button class="btn h-fit w-fit bg-primary-500" on:click={onFormSubmit}
					>Save Playlist</button
				>
			</div>
		</div>
	</div>
{/if}
