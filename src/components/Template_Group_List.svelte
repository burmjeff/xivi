<!-- TemplateGroup.svelte -->
<script lang="ts">
    import TemplateChannel from './TemplateChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, type PopupSettings } from '@skeletonlabs/skeleton';
    import {templateGroups} from '@xivi/stores/template_store';
    import {flip} from 'svelte/animate';
    import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
    import type { TemplateGroup } from '@xivi/data/template_entities';
    import {fade} from 'svelte/transition';
    import {cubicIn} from 'svelte/easing';

    let items: TemplateGroup[]

    const updateTemplateGroups = async () => {
        const response = await fetch('/api/template/groups/all');
        const data = await response.json();
        return data.templategroups;
    }

    onMount(async () => {
        const fetchedData = await updateTemplateGroups();
        templateGroups.set(fetchedData);
        items = $templateGroups
        });

    let templateGroupSettings: PopupSettings = {
        // Set the event as: click | hover | hover-click
        event: 'click',
        // Provide a matching 'data-popup' value.
        target: 'addTemplateGroupPopup'
    };

    async function addTemplateGroup() {
        const inputName = {name: (document.querySelector('.template_group_name input') as HTMLInputElement).value};
        if (inputName !== null) {
            try {
                const response = await fetch('/api/template/group', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(inputName)
                });
                if (response.ok) {
                    const fetchedData = await updateTemplateGroups();
                    templateGroups.set(fetchedData);
                    items = $templateGroups
                } else {
                    console.error('Error:', response.status, response.statusText);
                }
            } catch (error) {
                console.log('Error creating templateGroup:', error);
                return [];
            }
        }
    }

    const flipDurationMs = 300;
    let shouldIgnoreDndEvents = false;
    function handleDndConsider(e: CustomEvent<DndEvent<TemplateGroup>>) {
        console.warn(`got consider ${JSON.stringify(e.detail, null, 2)}`);
        const {trigger, id} = e.detail.info;
        if (trigger === TRIGGERS.DRAG_STARTED) {
            console.warn(`copying ${id}`);
            const idx = items.findIndex(item => item.id === Number(id));
            const newId = `${id}_copy_${Math.round(Math.random()*100000)}`;
			// the line below was added in order to be compatible with version svelte-dnd-action 0.7.4 and above 
			e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
            e.detail.items.splice(idx, 0, {...items[idx], id: Number(newId)});
            items = e.detail.items;
            shouldIgnoreDndEvents = true;
        }
        else if (!shouldIgnoreDndEvents) {
            items = e.detail.items;
        }
        else {
            items = [...items];
        }
    }
    function handleDndFinalize(e: CustomEvent<DndEvent<TemplateGroup>>) {
        console.warn(`got finalize ${JSON.stringify(e.detail, null, 2)}`);
        if (!shouldIgnoreDndEvents) {
            items = e.detail.items;
        }
        else {
            items = [...items];
            shouldIgnoreDndEvents = false;
        }
    }
</script>

<div class="card card-hover p-2">
    <section class="flex items-center space-x-4">
        <h1>Groups</h1>
        <button class="btn btn-sm variant-ringed-primary" use:popup={templateGroupSettings}>+ add new</button>
    </section>
    
    {#if $templateGroups.length > 0}
                <Accordion>
                    <section use:dndzone={{items, flipDurationMs}} on:consider={handleDndConsider} on:finalize={handleDndFinalize}>
                        {#each items as group(group.id)}
                            <div animate:flip="{{duration: flipDurationMs}}">
                                <AccordionItem key={group.id}>
                                    <svelte:fragment slot="summary"><h4>{group.name}</h4></svelte:fragment>
                                    <svelte:fragment slot="content">
                                        <TemplateChannel groupId={group.id} />
                                    </svelte:fragment>
                                </AccordionItem>
                                {#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
                                    <div in:fade={{duration:200, easing: cubicIn}} class='custom-shadow-item'>{group.name}</div>
                                {/if}
                            </div>
                        {/each}
                    </section>
                </Accordion>
    {:else}
        <p>No groups found</p>
    {/if}
</div>
<div class="card p-4 gap-4" data-popup="addTemplateGroupPopup">
	<h2>Add Template Group</h2>
    <div class="space-y-4">
        <label class="template_group_name">
            <span>Template Group Name</span>
            <input class="input" type="text" placeholder="Template Group Name" />
        </label>
        <label class="submit_button">
            <button class="btn bg-primary-500" on:click={addTemplateGroup}>Add Template Group</button>
        </label>
    </div>
</div>

<style>
    .custom-shadow-item {
		position: absolute;
		top: 0; left:0; right: 0; bottom: 0;
		visibility: visible;
		border: 2px dashed grey;
		background: lightblue;
		opacity: 0.6;
		margin: 0;
	}
</style>