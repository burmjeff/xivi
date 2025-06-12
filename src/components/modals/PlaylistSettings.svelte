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
		<header class="text-center text-2xl font-bold mb-2">Add Playlist</header>
	{:else}
		<header class="text-center text-2xl font-bold mb-2">Modify Playlist</header>
	{/if}
	<div class="space-y-4">
		<div class="form-group">
			<label class="block text-sm font-medium mb-2" for="playlist_name">
				Playlist Name
			</label>
			<input
				id="playlist_name"
				name="playlist_name"
				class="input w-full"
				type="text"
				bind:value={formData.name}
				placeholder="Enter playlist name"
				required
			/>
		</div>
		<div class="form-group">
			<label class="block text-sm font-medium mb-2" for="playlist_url">
				Playlist URL
			</label>
			<input
				id="playlist_url"
				name="playlist_url"
				class="input w-full"
				type="url"
				bind:value={formData.url}
				placeholder="https://example.com/playlist.m3u"
				required
			/>
		</div>
		<footer class="modal-footer flex justify-end gap-4 pt-4 border-t border-surface-600">
			<button class="btn preset-outlined-surface-500 min-w-20" onclick={onCancel}>
				Cancel
			</button>
			<button class="btn preset-filled-primary-500 min-w-20" onclick={onFormSubmit}>
				Save
			</button>
		</footer>
	</div>
</div>

<style>
	.modal-playlist {
		min-width: 400px;
		padding: 1.5rem;
	}

	.form-group {
		display: flex;
		flex-direction: column;
	}

	.form-group label {
		color: var(--color-surface-200);
		font-weight: 500;
	}

	.form-group input {
		transition: all 0.2s ease;
	}

	.form-group input:focus {
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	}

	.modal-footer {
		margin-top: 2rem;
	}

	.modal-footer button {
		font-weight: 500;
		padding: 0.75rem 1.5rem;
	}

	/* Mobile responsive */
	@media (max-width: 640px) {
		.modal-playlist {
			min-width: unset;
			padding: 1rem;
		}

		.modal-footer {
			flex-direction: column;
			gap: 0.75rem;
		}

		.modal-footer button {
			width: 100%;
		}
	}
</style>