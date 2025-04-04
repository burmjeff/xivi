<!-- PlaylistSettings.svelte -->

<script lang="ts">
	const { parent, isNew, id, name, url } = $props();

	let formData: {
		name: string;
		url: string;
	} = $state({
		name: isNew ? '' : name,
		url: isNew ? '' : url
	});

	async function onFormSubmit(): Promise<void> {
		console.log('Form submitted with data:', formData);
		parent.onClose(formData);
	}

	function onCancel(): void {
		console.log('Cancel button clicked');
		parent.onClose();
	}
</script>

<div class="modal-playlist">
	{#if isNew}
		<header class="justify-center text-center text-2xl font-bold">Add Playlist</header>
	{:else}
		<header class="justify-center text-center text-2xl font-bold">Modify Playlist</header>
	{/if}
	<div class="space-y-4">
		<label class="playlist_name p-2">
			<span>Playlist Name</span>
			<input
				name="playlist_name"
				class="input"
				type="text"
				bind:value={formData.name}
				placeholder="Playlist Name"
			/>
		</label>
		<label class="playlist_url p-2">
			<span>Playlist URL</span>
			<input
				name="playlist_url"
				class="input"
				type="text"
				bind:value={formData.url}
				placeholder="https://example.com/xivi.m3u"
			/>
		</label>
		<footer class="modal-footer flex justify-end gap-4">
			<button class="btn preset-outlined-surface-500" onclick={onCancel}>Cancel</button>
			<button class="btn preset-filled-primary-500" onclick={onFormSubmit}>Save</button>
		</footer>
	</div>
</div>