<!-- Player.svelte -->

<script lang="ts">
	import { onMount, type SvelteComponent } from 'svelte';
    import { getModalStore } from '@skeletonlabs/skeleton';
    import '@mux/mux-video';
    import xivi from '$lib/assets/xivi.png';

    export let parent: SvelteComponent;

    const modalStore = getModalStore();
    let stream = $modalStore[0].meta.stream
    let name = $modalStore[0].meta.name

</script>

{#if $modalStore[0]}
    <mux-video
        src={stream}
        type="video/mp4"
        controls
        preload="metadata"
        autoplay
        on:play
        on:pause
        on:ended>
    </mux-video>
{/if}