<!-- Template.svelte -->
<script lang="ts">
    import TemplateGroup from './TemplateGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, type PopupSettings } from '@skeletonlabs/skeleton';
    import { templates } from '@xivi/stores/template_store';

    onMount(async () => {
        fetch('/api/templates')
        .then(response => response.json())
        .then(data => {
            console.log('Fetched data:', data);
            templates.set(data.templates);
        }).catch(error => {
            console.log('Error fetching data:', error);
            return [];
        });
        });
    
    let templateSettings: PopupSettings = {
        // Set the event as: click | hover | hover-click
        event: 'click',
        // Provide a matching 'data-popup' value.
        target: 'addTemplatePopup'
    };

    async function addTemplate() {
        const inputName = {name: (document.querySelector('.template_name input') as HTMLInputElement).value};
        if (inputName !== null) {
            try {
                const response = await fetch('/api/template', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(inputName)
                });
                const data = await response.json();
                console.log('Created template', data);
                templates.set(data.templates);
            } catch (error) {
                console.log('Error creating template:', error);
                return [];
            }
        }
    }
</script>

<div class="card card-hover p-2 px-2">
    <section class="flex items-center space-x-4">
        <h1>Templates</h1>
        <button class="btn btn-sm variant-ringed-primary" use:popup={templateSettings}>+ add new</button>
    </section>
    
    {#if $templates.length > 0}
        <Accordion>
            {#each $templates as template}
                <AccordionItem key={template.id}>
                    <svelte:fragment slot="lead">{template.id}</svelte:fragment>
                    <svelte:fragment slot="summary"><h4>{template.name}</h4></svelte:fragment>
                    <svelte:fragment slot="content">
                        <TemplateGroup templateId={template.id} />
                    </svelte:fragment>
                </AccordionItem>
            {/each}
        </Accordion>
    {:else}
        <p>No templates found</p>
    {/if}
</div>
<div class="card p-4 gap-4" data-popup="addTemplatePopup">
	<h2>Add Template</h2>
    <div class="space-y-4">
        <label class="template_name">
            <span>Template Name</span>
            <input class="input" type="text" placeholder="Template Name" />
        </label>
        <label class="submit_button">
            <button class="btn bg-primary-500" on:click={addTemplate}>Add Template</button>
        </label>
    </div>
</div>