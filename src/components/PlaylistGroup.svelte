<!-- PlaylistGroup.svelte -->
<script lang="ts">
    import PlaylistChannel from './PlaylistChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, ListBox, ListBoxItem, type PopupSettings } from '@skeletonlabs/skeleton';
    import {playlistGroups} from '@xivi/stores/playlist_store';
    import {templates} from '@xivi/stores/template_store';
    import { Modal, getModalStore } from '@skeletonlabs/skeleton';
    import type { ModalSettings, ModalComponent, ModalStore } from '@skeletonlabs/skeleton';
    import IconParkOutlineTransferData from '~icons/icon-park-outline/transfer-data'
  
    export let playlistId: number;
    const modalStore = getModalStore();
  
    onMount(async () => {
        fetch(`/api/playlist/${playlistId}/groups`)
        .then(response => response.json())
        .then(data => {
            console.log(data);
            playlistGroups.set(data.playlistgroups);
        }).catch(error => {
            console.log(error);
            return [];
        });
        });

    function modalPrompt(groupId: number): void {
		const prompt: ModalSettings = {
			type: 'prompt',
			title: 'Convert Playlist Group to Template Group',
			body: 'Enter new template group name in field below.',
			value: 'Example Template',
			valueAttr: { type: 'text', minlength: 1, maxlength: 20, required: true },
			response: (templateName: string) => {
				if (templateName) convertGroup(templateName, groupId);
			},
            buttonTextCancel: 'Cancel',
		    buttonTextSubmit: 'Submit',
		};
		modalStore.trigger(prompt);
	}

    async function convertGroup(templateName: string, groupId: number) {
        if (templateName !=='') {
            const newTemplate = {
                name: templateName
            };
            try {
                const response = await fetch(`/api/playlist/${playlistId}/group/${groupId}/convert`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(newTemplate)
                });
                const data = await response.json();
                console.log('Created template group:', data);
                templates.set(data.templates);
            } catch (error) {
                console.log('Error creating template group:', error);
                return [];
            }
        }
    }
  </script>

  <Accordion>
    {#each $playlistGroups as group}
        <section class="flex items-start space-x-4 ">
            <AccordionItem key={group.id}>
                <svelte:fragment slot="summary"><h4>{group.name}</h4></svelte:fragment>
                <svelte:fragment slot="content">
                <PlaylistChannel playlistId={playlistId} groupId={group.id} />
                </svelte:fragment>
            </AccordionItem>
            <button class="btn-icon variant-filled-surface w-1" on:click={() => modalPrompt(group.id)}><i><IconParkOutlineTransferData/></i></button>
        </section>
    {/each}
</Accordion>
