<script lang="ts">
	// Import styles.
	import 'vidstack/player/styles/default/theme.css';
	import 'vidstack/player/styles/default/layouts/video.css';
	// Register elements.
	import 'vidstack/player';
	import 'vidstack/player/layouts';
	import 'vidstack/player/ui';

	import { onMount, type SvelteComponent } from 'svelte';
	import { getModalStore } from '@skeletonlabs/skeleton';
	import xivi from '$lib/assets/xivi.png';
	import {
		type MediaProviderSetupEvent,
		type MediaProviderAdapter,
		MediaRemoteControl,
		isGoogleCastProvider
	} from 'vidstack';

	//export let parent: SvelteComponent;

	const modalStore = getModalStore();
	let videoUrl = $modalStore[0].meta.stream;
	let name = $modalStore[0].meta.name;
	let player: HTMLElement;
	const remote = new MediaRemoteControl();

	remote.disableCaptions();

	onMount(async () => {
		player.addEventListener('provider-setup', (event) => {
			const provider = (<CustomEvent>event).detail;
			if (provider?.type === 'google-cast') {
				// Google Cast remote player.
				provider.player;
				// Google Cast context.
				provider.cast;
				// Google Cast session.
				provider.session;
				// Google Cast media info.
				provider.media;
				// Whether the session belongs to this provider.
				provider.hasActiveSession;
			}
		});
	});
</script>

<media-player
	bind:this={player}
	title={name}
	src={videoUrl}
	class="player"
	streamType="live"
	crossorigin
	autoPlay
>
	<media-provider>
		<media-poster class="vds-poster" src={xivi} alt={name} />
	</media-provider>
	<!-- Layouts -->
	<media-video-layout />
</media-player>

<style lang="postcss">
	.player {
		max-height: 100vh;
		max-width: 100vh;
		height: 57vh;
		width: 100vh;
		aspect-ratio: 16 /9;
		--brand-color: #f5f5f5;
		--focus-color: #4e9cf6;

		--video-brand: var(--brand-color);
		--video-focus-ring-color: var(--focus-color);
		--video-border-radius: 2px;

		/* 👉 https://vidstack.io/docs/player/components/layouts/default#css-variables for more. */
	}
</style>
