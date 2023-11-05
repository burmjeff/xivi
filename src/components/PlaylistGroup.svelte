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
    import {flip} from 'svelte/animate';
    import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
    import type { PlaylistGroup } from '@xivi/data/playlist_entities';
    import {fade} from 'svelte/transition';
    import {cubicIn} from 'svelte/easing';
  
    export let playlistId: number;
    const modalStore = getModalStore();

    const updatePlaylistGroups = async () => {
        const response = await fetch(`/api/playlist/${playlistId}/groups`);
        const data = await response.json();
        return data.playlistgroups;
    }
  
    onMount(async () => {
        const fetchedData = await updatePlaylistGroups();
        playlistGroups.set(fetchedData);
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

    const flipDurationMs = 300;
    function handleDndConsider(e: CustomEvent<DndEvent<PlaylistGroup>>) {
        console.warn(`got consider ${JSON.stringify(e.detail, null, 2)}`);
        const {trigger, id} = e.detail.info;
        if (trigger === TRIGGERS.DRAG_STARTED) {
            //console.warn(`copying ${id}`);
            //const idx = items.findIndex(item => item.id === Number(id));
            //const newId = `${id}_copy_${Math.round(Math.random()*100000)}`;
			// the line below was added in order to be compatible with version svelte-dnd-action 0.7.4 and above 
			e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
            //e.detail.items.splice(idx, 0, {...items[idx], id: Number(newId)});
            $playlistGroups = e.detail.items;
        }
        else {
            $playlistGroups = e.detail.items;
        }
    }
    function handleDndFinalize(e: CustomEvent<DndEvent<PlaylistGroup>>) {
        console.warn(`got finalize ${JSON.stringify(e.detail, null, 2)}`);
        $playlistGroups = e.detail.items;
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
