<!-- TemplateSettings.svelte -->

<script lang="ts">
	const { parent, isNew, id, name } = $props();

	let formData: {
		name: string;
	} = $state({
		name: isNew ? '' : name
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

<div class="modal-template">
	{#if isNew}
		<header class="justify-center text-center text-2xl font-bold">Add Template</header>
	{:else}
		<header class="justify-center text-center text-2xl font-bold">Modify Template</header>
	{/if}
	<div class="space-y-4">
		<label class="template_name p-2">
			<span>Template Name</span>
			<input
				class="input"
				type="text"
				bind:value={formData.name}
				placeholder="Template Name"
			/>
		</label>
		<footer class="modal-footer flex justify-end gap-4">
			<button class="btn preset-outlined-surface-500" onclick={onCancel}>Cancel</button>
			<button class="btn preset-filled-primary-500" onclick={onFormSubmit}>Save</button>
		</footer>
	</div>
</div>
