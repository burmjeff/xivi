<!-- TemplateGroup.svelte -->
<script lang="ts">
    import TemplateChannel from './TemplateChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, getModalStore, type ModalSettings } from '@skeletonlabs/skeleton';
    import { templates } from '@xivi/stores/template_store';
    import {templateGroups} from '@xivi/stores/template_store';
    import type { TemplateGroup } from '@xivi/data/template_entities';
    import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
    import {flip} from 'svelte/animate';
    import {fade} from 'svelte/transition';
    import {cubicIn} from 'svelte/easing';
    import Icon from '@iconify/svelte';
    
    const modalStore = getModalStore();
    export let templateId: number;
    export let templateIdx: number
    let dndTypeGroups = "groups";
    let shouldIgnoreDndEvents = false;
    const flipDurationMs = 150;
    let dndItem: TemplateGroup;
	let dndIdx: number

    const updateTemplateGroups = async () => {
		const response = await fetch(`/api/template/${templateId}/groups`);
		const data = await response.json();
		return data.templategroups;
	};
  
    onMount(async () => {
        $templates[templateIdx].groups = []
        const fetchedData = await updateTemplateGroups();
        if (typeof fetchedData !== 'undefined') {
            $templates[templateIdx].groups = fetchedData;
            $templates[templateIdx].groups = $templates[templateIdx].groups;
        }
    });

    async function addGroup(groupId: string) {
        try {
            const response = await fetch(`/api/template/${templateId}/group/${groupId}/item`, {
                method: 'POST'
            });
            if (await response.ok) {
                let newGroup = $templateGroups.find(item => item.id === groupId)
                console.log('Added template group:', newGroup);
                const fetchedData = await updateTemplateGroups();
                if (typeof fetchedData !== 'undefined') {
                    $templates[templateIdx].groups = fetchedData;
                    $templates[templateIdx].groups = $templates[templateIdx].groups;
                }
            } else console.log('Error adding template group:',response)
        } catch (error) {
            console.log('Error adding template group:', error);
        }
	}

    function deletePrompt(groupId: string) {
		const modal: ModalSettings = {
			type: 'confirm',
            title: 'Please Confirm',
            body: 'Are you sure you wish to remove this group from template?',
            // TRUE if confirm pressed, FALSE if cancel pressed
            response: (r: boolean) => {
				if (r) deleteTemplateGroup(groupId);
			},
		};
		modalStore.trigger(modal);
	}
    
    //TODO COLLAPSE ACCORDIION ITEM BEFORE DELETE
    async function deleteTemplateGroup(groupId: string) {
		try {
			const response = await fetch(`/api/template/${templateId}/group/${groupId}/item`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Removing template group:', data);
			$templates[templateIdx].groups = $templates[templateIdx].groups.filter(t => t.id != groupId)
			modalStore.close();
		} catch (error) {
			console.log('Error removing template group:', error);
			return;
		}
	}

    function handleDndConsider(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const {trigger, id} = e.detail.info;
		//e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $templates[templateIdx].groups.findIndex(item => item.id === id);
			dndItem =  $templates[templateIdx].groups[dndIdx];
            e.detail.items[dndIdx].itemOpen = false;
            $templates[templateIdx].groups[dndIdx].itemOpen = false;
			$templates[templateIdx].groups = e.detail.items
			shouldIgnoreDndEvents = true;
		}
        else if (!shouldIgnoreDndEvents) {
            $templates[templateIdx].groups = e.detail.items;
        }
        else {
            $templates[templateIdx].groups = [...$templates[templateIdx].groups];
        }
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const {trigger, id} = e.detail.info;
        if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreDndEvents) {
            e.detail.items = e.detail.items.filter(item => !item.isDragged);
            addGroup(id)
            $templates[templateIdx].groups = e.detail.items
            shouldIgnoreDndEvents = false;
        }
        else if (!shouldIgnoreDndEvents) {
            $templates[templateIdx].groups = e.detail.items
        }
        else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER){
			e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx,0, dndItem)
            $templates[templateIdx].groups = e.detail.items
            shouldIgnoreDndEvents = false;
        } else {
            $templates[templateIdx].groups = e.detail.items
            shouldIgnoreDndEvents = false;
        }
    }
    function transformDraggedElement(draggedEl: HTMLElement | undefined, data: Item | undefined, index: number | undefined) {
        if (!shouldIgnoreDndEvents) data!.isDragged = true
	}
    
  </script>

{#if $templates[templateIdx].groups != null}
    <Accordion>
        <section use:dndzone={{items: $templates[templateIdx].groups, flipDurationMs, type: dndTypeGroups, transformDraggedElement}} on:consider={handleDndConsider} on:finalize={handleDndFinalize}>
            {#if $templates[templateIdx].groups.length > 0}
                    {#each $templates[templateIdx].groups as group, groupIdx (group.id)}
                        <div id="animate" animate:flip={{duration: flipDurationMs}}>
                            <AccordionItem class="card mb-1" key={groupIdx} bind:open={group.itemOpen}>
                                <svelte:fragment slot="summary">
                                    <div class="flex flex-row items-center">
                                        <h4 class="text-lg">{group.name}</h4>
                                        <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" on:click={() => {group.itemOpen = true, deletePrompt(group.id)}}>
                                            <Icon icon="icon-park-outline:delete" width="18" height="18"/>
                                        </button>
                                    </div>
                                </svelte:fragment>
                                <svelte:fragment slot="content">
                                    <TemplateChannel groupId={group.id} groupIdx={groupIdx}/>
                                </svelte:fragment>
                            </AccordionItem>
                            {#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
                                <div in:fade={{duration:200, easing: cubicIn}} class='custom-shadow-item'>{group.name}</div>
                            {/if}
                        </div>
                    {/each}        
            {:else}
                <p>No groups found</p>
            {/if}
        </section>
    </Accordion>
{/if}

<style>
    .custom-shadow-item {
		position: absolute;
		top: 0; left:0; right: 0; bottom: 0;
		visibility: visible;
		border: 3px dashed grey;
		background: lightblue;
		opacity: 0.6;
		margin: 0;
	}
    #animate {
		position: relative;
		text-align: center;
	}
</style>