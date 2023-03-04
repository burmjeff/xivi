<!-- Template.svelte -->
<script lang="ts">
    import TemplateGroup from './TemplateGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
    import { templates } from '../stores/template_store';

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
</script>

<div class="card card-hover p-4">Templates
    {#if $templates.length > 0}
        <Accordion>
            {#each $templates as template}
                <AccordionItem key={template.id}>
                    <svelte:fragment slot="lead">{template.id}</svelte:fragment>
                    <svelte:fragment slot="summary"><h3>{template.name}</h3></svelte:fragment>
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