<script lang="ts">
    import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
    import { Table } from '@skeletonlabs/skeleton';
    import type { TableSource } from '@skeletonlabs/skeleton';
    import { tableMapperValues } from '@skeletonlabs/skeleton';

    
    const sourceData = [
        { position: 1, name: 'News', channel_count: 25 },
        { position: 2, name: 'Entertainment', channel_count: 120 },
        { position: 3, name: 'Sports', channel_count: 50 },
    ];
			
    const tableSimple: TableSource = {
        // A list of heading labels.
        head: ['Position','Group Name', 'Channel Count'],
        // The data visibly shown in your table body UI.
        body: tableMapperValues(sourceData, ['position', 'name', 'channel_count']),
        // Optional: The data returned when interactive is enabled and a row is clicked.
        meta: tableMapperValues(sourceData, ['position', 'name', 'channel_count']),
        // Optional: A list of footer labels.
        foot: ['Total Channels', '', '<code>450</code>']
    };
</script>

<div class="card card-hover p-4">Templates
    <Accordion>
        <AccordionItem open>
            <svelte:fragment slot="lead">(icon)</svelte:fragment>
            <svelte:fragment slot="summary"><h3>(Template Name)</h3></svelte:fragment>
            <svelte:fragment slot="content">
                <Table source={tableSimple} interactive={true}/>
            </svelte:fragment>
        </AccordionItem>
        <AccordionItem>
            <svelte:fragment slot="lead">(icon)</svelte:fragment>
            <svelte:fragment slot="summary"><h3>(Template Name)</h3></svelte:fragment>
            <svelte:fragment slot="content">(content)</svelte:fragment>
        </AccordionItem>
        <!-- ... -->
    </Accordion>
</div>