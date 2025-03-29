<!-- PlaylistSettings.svelte -->

<script lang="ts">
	import { Modal } from '@skeletonlabs/skeleton-svelte';

	let { modalOpen = $bindable(), parent, isNew, id, name, url } = $props<{
		modalOpen: boolean;
		parent: any;
		isNew: boolean;
		id: number;
		name: string;
		url: string;
	}>();

	let formData: {
		name: string;
		url: string;
	} = $state({
		name: '',
		url: ''
	});

	if (!isNew) {
		formData = {
			name: name,
			url: url
		};
	} else {
		formData = {
			name: '',
			url: ''
		};
	}

	async function onFormSubmit(): Promise<void> {
		parent.onClose(formData);
		modalClose();
	}

	function modalClose() {
		modalOpen = false;
	}
</script>

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl max-w-screen-sm"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
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
				<button class="btn h-fit w-fit bg-primary-500" onclick={onFormSubmit}
					>Save Playlist</button
				>
			</div>
		</div>
	</div>
	{/snippet}
</Modal>
