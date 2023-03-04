<!-- TemplateChannel.svelte -->
<script lang="ts">
    import { onMount } from 'svelte';
    import { templateChannels } from '@xivi/stores/template_store';
  
    export let groupId: number;
  
    let channels: typeof templateChannels

    onMount(async () => {
        fetch(`/api/template/group/${groupId}/channels`)
        .then(response => response.json())
        .then(data => {
            console.log(data);
            templateChannels.set(data.templatechannels);
        }).catch(error => {
            console.log(error);
            return [];
        });
        });

</script>
  
<table class="table">
    <thead>
        <tr>
            <th>Logo</th>
            <th>Name</th>
            <th>tvg-id</th>
        </tr>
    </thead>
    <tbody>
        {#each $templateChannels as channel}
            <tr>
                <td>{channel.logo}</td>
                <td>{channel.name}</td>
                <td>{channel.tvgid}</td>
            </tr>
        {/each}
    </tbody>
</table>