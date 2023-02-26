<script lang="ts">
    import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
    import { Table } from '@skeletonlabs/skeleton';
    import type { TableSource } from '@skeletonlabs/skeleton';
    import { tableMapperValues } from '@skeletonlabs/skeleton';

    
    const sourceData = [
        { position: 1, name: 'Fox News', tvgid: "foxnews.us" },
        { position: 2, name: 'CNN', tvgid: "cnn.us" },
        { position: 3, name: 'BBC', tvgid: "bbc.us" },
    ];
			
    const tableSimple: TableSource = {
        // A list of heading labels.
        head: ['Position','Channel Name', 'tvg-id'],
        // The data visibly shown in your table body UI.
        body: tableMapperValues(sourceData, ['position', 'name', 'tvgid']),
        // Optional: The data returned when interactive is enabled and a row is clicked.
        meta: tableMapperValues(sourceData, ['position', 'name', 'tvgid']),
        // Optional: A list of footer labels.
        foot: ['Total Channels', '', '<code>450</code>']
    };
</script>

<div class="card card-hover p-4">Playlists
    <Accordion>
        <AccordionItem open>
            <svelte:fragment slot="lead">(icon)</svelte:fragment>
            <svelte:fragment slot="summary"><h3>(Group Name)</h3></svelte:fragment>
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