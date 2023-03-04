<!-- TemplateGroup.svelte -->
<script lang="ts">
    import TemplateChannel from './TemplateChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
    import {templateGroups} from '@xivi/stores/template_store';
  
    export let templateId: number;
  
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

  <Accordion>
    {#each $templateGroups as group}
        <AccordionItem>
            <svelte:fragment slot="lead">{group.id}</svelte:fragment>
            <svelte:fragment slot="summary"><h3>{group.name}</h3></svelte:fragment>
            <svelte:fragment slot="content">
              <TemplateChannel groupId={group.id} />
            </svelte:fragment>
        </AccordionItem>
    {/each}
</Accordion>