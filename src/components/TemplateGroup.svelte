<!-- TemplateGroup.svelte -->
<script lang="ts">
    import TemplateChannel from './TemplateChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
    import {templateGroups} from '@xivi/stores/template_store';
    
    export let templateId: string;
  
    onMount(async () => {
        fetch(`/api/template/${templateId}/groups`)
        .then(response => response.json())
        .then(data => {
            console.log(data);
            templateGroups.set(data.templategroups);
        }).catch(error => {
            console.log(error);
            return [];
        });
    });
    
  </script>

<Accordion class="object-fit: contain overflow-auto" type='asd'>
    {#each $templateGroups as group, groupIdx (group.id)}
        <section class="items-start space-x-4 ">
            <AccordionItem>
                <svelte:fragment slot="lead">{group.id}</svelte:fragment>
                <svelte:fragment slot="summary"><h3>{group.name}</h3></svelte:fragment>
                <svelte:fragment slot="content">
                <TemplateChannel groupId={group.id} groupIdx={groupIdx}/>
                </svelte:fragment>
            </AccordionItem>
        </section>
    {/each}
</Accordion>