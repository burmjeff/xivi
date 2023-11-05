<!-- Settings.svelte -->

<script lang="ts">
    import { templateChannels } from '@xivi/stores/template_store';
    import IconParkOutlineSaveOne from '~icons/icon-park-outline/save-one'
    import type { SvelteComponent } from 'svelte';
	import { getModalStore, FileButton } from '@skeletonlabs/skeleton';

	export let parent: SvelteComponent;
	const modalStore = getModalStore();
  
    const formData = {
		name: $templateChannels[$modalStore[0].meta.channelId].name,
		tvgid: $templateChannels[$modalStore[0].meta.channelId].tvgid,
		logo: $templateChannels[$modalStore[0].meta.channelId].logo
	};

    function onUploadHandler(e: Event): void {
	    console.log('file data:', e);
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
                <FileButton name="files" on:change={onUploadHandler}>Upload</FileButton>
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