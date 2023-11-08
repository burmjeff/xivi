<!-- Settings.svelte -->

<script lang="ts">
	import { templateGroups } from '@xivi/stores/template_store';
	import IconParkOutlineSaveOne from '~icons/icon-park-outline/save-one';
	import IconParkOutlineDelete from '~icons/icon-park-outline/delete';
	import type { SvelteComponent } from 'svelte';
	import { getModalStore, FileButton } from '@skeletonlabs/skeleton';
	import type { Logo } from '@xivi/data/logo_entities';
	import xivi from '$lib/assets/xivi.png';

	export let parent: SvelteComponent;
	const modalStore = getModalStore();
	let files: FileList;
	let groupIdx = $modalStore[0].meta.groupIdx
	let channelIdx = $modalStore[0].meta.channelIdx
	let isNew = $modalStore[0].meta.isNew

	let formData: {
		id: number,
		name: string,
		tvgid: string,
		logoid: number,
		logo: string
	}

	if (!isNew) {
		formData = {
				id: $templateGroups[groupIdx].channels[channelIdx].id,
				name: $templateGroups[groupIdx].channels[channelIdx].name,
				tvgid: $templateGroups[groupIdx].channels[channelIdx].tvgid,
				logoid: $templateGroups[groupIdx].channels[channelIdx].logoid,
				logo: $templateGroups[groupIdx].channels[channelIdx].logo
			};
	} else {
		formData = {
			id: 0,
			name: "",
			tvgid: "",
			logoid: 0,
			logo: xivi
		}
	}

	const toBase64 = (file: File) =>
		new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.readAsDataURL(file);
			reader.onload = () => resolve(reader.result);
			reader.onerror = reject;
		});

	async function uploadImage() {
		fetch('/api/logo', {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json'
			},
			body: JSON.stringify({image: formData.logo})
		})
			.then((response) => response.json())
			.then((data) => {
				console.log('Uploaded logo:', data);
				formData.logoid = data.logo.id;
				formData.logo = data.logo.image;
			})
			.catch((error) => {
				console.log('Error uploading logo:', error);
			});
	}

	function onUploadHandler(e: Event): void {
		console.log('file data:', e);
		if (files) {
			const result = String(toBase64(files[0]));
			if (result) {
				formData.logo = result
			}
		}
	}

	function onFormSubmit(): void {
		if ($modalStore[0].response) {
			uploadImage();
			$modalStore[0].response(formData);
		}
		modalStore.close();
	}

	async function deleteChannel() {
		try {
			const response = await fetch(`/api/template/channel/${$templateGroups[groupIdx].channels[channelIdx].id}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted template channel:', data);
			$templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels.filter(t => t.id != formData.id)
			modalStore.close();
		} catch (error) {
			console.log('Error deleting template channel:', error);
			return;
		}
	}
	
</script>

{#if $modalStore[0]}
	<div class="modal-channel-settings card p-4 w-modal shadow-xl space-y-4">
		{#if isNew}
			<header class="text-2xl font-bold text-center justify-center">Add Channel</header>
		{:else}
			<header class="text-2xl font-bold text-center justify-center">Channel Settings</header>
		{/if}
		<form class="modal-form border border-surface-500 p-4 space-y-4 rounded-container-token">
			<label class="channel_name">
				<span>Channel Name</span>
				<input
					class="input variant-form-material"
					type="text"
					bind:value={formData.name}
					placeholder=""
				/>
			</label>
			<label class="channel_tvgid">
				<span>Channel tvgid</span>
				<input
					class="input variant-form-material"
					type="text"
					bind:value={formData.tvgid}
					placeholder=""
				/>
			</label>
			<div class="channel_logo">
				<span>Channel Logo</span>
				<div class="grid grid-cols-2 p-2 gap-10 w-64 items-center">
					<img class="w-fit" src={formData.logo} alt="Logo" />
					<FileButton
						name="files"
						bind:files
						accept=".png,.jpg,.webp,.svg"
						on:change={onUploadHandler}>Upload</FileButton
					>
				</div>
			</div>
		</form>
		<footer class="modal-footer {parent.regionFooter}">
			{#if !isNew}
				<button class="btn variant-ghost-error" on:click={deleteChannel}>
					<i><IconParkOutlineDelete/></i>
					<span>Delete</span>
				</button>
			{/if}
			<button class="btn {parent.buttonNeutral}" on:click={parent.onClose}>
				{parent.buttonTextCancel}</button>
			<button class="btn {parent.buttonPositive}" on:click={onFormSubmit}>
				<i><IconParkOutlineSaveOne/></i>
				<span>Save Changes</span>
			</button>
			
		</footer>
	</div>
{/if}
