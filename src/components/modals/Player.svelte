<script lang="ts">
    // Import styles.
    import 'vidstack/player/styles/default/theme.css';
    import 'vidstack/player/styles/default/layouts/video.css';
    // Register elements.
    import 'vidstack/player';
    import 'vidstack/player/layouts';
    import 'vidstack/player/ui';

    import { onMount, afterUpdate, type SvelteComponent } from 'svelte';
    import { getModalStore } from '@skeletonlabs/skeleton';
	  import xivi from '$lib/assets/xivi.png';
    import Hls from 'hls.js';

    export let parent: SvelteComponent;

    const modalStore = getModalStore();
    let videoUrl = $modalStore[0].meta.stream
    let name = $modalStore[0].meta.name
    let player: HTMLVideoElement
</script>

<media-player
bind:this={player}
title={name}
src={videoUrl}
class="player"
streamType="ll-live"
crossorigin
playsinline
autoPlay
>
<media-provider>
  <media-poster
    class="vds-poster"
    src="https://image.mux.com/VZtzUzGRv02OhRnZCxcNg49OilvolTqdnFLEqBsTwaxU/thumbnail.webp?time=268&width=1200"
    alt="Girl walks into campfire with gnomes surrounding her friend ready for their next meal!"
  />
</media-provider>
<!-- Layouts -->
<media-video-layout
  thumbnails="https://image.mux.com/VZtzUzGRv02OhRnZCxcNg49OilvolTqdnFLEqBsTwaxU/storyboard.vtt"
/>
</media-player>

<style lang="postcss">
.player {
  --brand-color: #f5f5f5;
  --focus-color: #4e9cf6;

  --video-brand: var(--brand-color);
  --video-focus-ring-color: var(--focus-color);
  --video-border-radius: 2px;

  /* 👉 https://vidstack.io/docs/player/components/layouts/default#css-variables for more. */

  &[data-view-type='video'] {
    aspect-ratio: 16 /9;
  }
}
</style>