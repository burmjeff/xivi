<!-- TemplateGroup.svelte -->
<script lang="ts">
    import TemplateChannel from './TemplateChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, type PopupSettings } from '@skeletonlabs/skeleton';
    import {templateGroups} from '@xivi/stores/template_store';
  
    onMount(async () => {
        fetch(`/api/template/groups/all`)
        .then(response => response.json())
        .then(data => {
            console.log(data);
            templateGroups.set(data.templategroups);
        }).catch(error => {
            console.log(error);
            return [];
        });
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
                const data = await response.json();
                console.log('Created template group:', data);
                templateGroups.set(data.templateGroups);
            } catch (error) {
                console.log('Error creating templateGroup:', error);
                return [];
            }
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
            {#each $templateGroups as group}
                <AccordionItem>
                    <svelte:fragment slot="lead">{group.id}</svelte:fragment>
                    <svelte:fragment slot="summary"><h4>{group.name}</h4></svelte:fragment>
                    <svelte:fragment slot="content">
                        <TemplateChannel groupId={group.id} />
                    </svelte:fragment>
                </AccordionItem>
            {/each}
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