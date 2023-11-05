<!-- Settings.svelte -->

<script lang="ts">
    import { templateChannels } from '@xivi/stores/template_store';
    import IconParkOutlineSaveOne from '~icons/icon-park-outline/save-one'
    import type { SvelteComponent } from 'svelte';
	import { getModalStore, FileButton } from '@skeletonlabs/skeleton';
	import { logos } from '@xivi/stores/logo_store';
	import type { Logo, LogoUpload } from '@xivi/data/logo_entities';

	export let parent: SvelteComponent;
	const modalStore = getModalStore();
    let files: FileList;
  
    const formData = {
		name: $templateChannels[$modalStore[0].meta.channelId].name,
		tvgid: $templateChannels[$modalStore[0].meta.channelId].tvgid,
		logo: $templateChannels[$modalStore[0].meta.channelId].logo
	};

    const toBase64 = (file: File) => new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.readAsDataURL(file);
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
    });

    async function uploadImage() {
        if (files) {
            const result = String(await toBase64(files[0]));
            let logoUpload: LogoUpload;

            if (result) {
                logoUpload = {
                    type: files[0].type,
                    image: result,
                };
                window.console.log('Uploading Logo: ', logoUpload);

                fetch('/api/logo', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(logoUpload)
                })
                .then((response) => response.json())
                .then((data) => {
                    console.log('Uploaded logo:', data);
                    //logos.update(data.logo)
                    formData.logo = data.logo.img
                })
                .catch((error) => {
                    console.log('Error uploading logo:', error);
                    return [];
                });
                
            }
        }
	}

    function onUploadHandler(e: Event): void {
	    console.log('file data:', e);
        uploadImage();
    }

    function onFormSubmit(): void {
        if ($modalStore[0].response) $modalStore[0].response(formData);
		modalStore.close();
    }
</script>

{#if $modalStore[0]}
<div class="modal-channel-settings card p-4 w-modal shadow-xl space-y-4">
    <header class="text-2xl font-bold text-center justify-center">Channel Settings</header>
    <form class="modal-form border border-surface-500 p-4 space-y-4 rounded-container-token">
        <label class="channel_name">
            <span>Channel Name</span>
            <input class="input variant-form-material" type="text" bind:value={formData.name} placeholder=""/>
        </label>
        <label class="channel_tvgid">
            <span>Channel tvgid</span>
            <input class="input variant-form-material" type="text" bind:value={formData.tvgid} placeholder="" />
        </label>
        <div class="channel_logo">
            <span>Channel Logo</span>
            <div class="grid grid-cols-2 p-2 gap-10 w-64 items-center">
                <img class="w-fit" src={formData.logo} alt="Logo" />
                <FileButton name="files" bind:files={files} accept=".png,.jpg,.webp,.svg" on:change={onUploadHandler}>Upload</FileButton>
            </div>
        </div>
    </form>
    <footer class="modal-footer {parent.regionFooter}">
        <button class="btn {parent.buttonNeutral}" on:click={parent.onClose}>{parent.buttonTextCancel}</button>
        <button class="btn {parent.buttonPositive}" on:click={onFormSubmit}>
            <i><IconParkOutlineSaveOne/></i>
            <span>Save Changes</span>
        </button>
    </footer>
</div>
{/if}