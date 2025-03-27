<!-- Player.svelte -->

<script lang="ts">
	// Import styles.
	import 'vidstack/player/styles/default/theme.css';
	import 'vidstack/player/styles/default/layouts/video.css';
	// Register elements.
	import 'vidstack/player';
	import 'vidstack/player/layouts';
	import 'vidstack/player/ui';
	import { isHLSProvider, MediaRemoteControl, type MediaCanPlayEvent, type MediaProviderChangeEvent } from 'vidstack';
	import type { MediaPlayerElement } from 'vidstack/elements';
	import { Modal } from '@skeletonlabs/skeleton-svelte';
	import { onMount } from 'svelte';
	import xivi from '@xivi/lib/assets/xivi.png';

	let { modalOpen = $bindable(), parent, name, stream } = $props<{
		modalOpen: boolean;
		parent: any;
		name: string;
		stream: string;
	}>();

	let videoUrl = stream;
	let player: HTMLElement | undefined = $state();
	const remote = new MediaRemoteControl();
	let boolTrue = true;

	remote.disableCaptions();

	onMount(() => {
		if (!player) return;

		player.addEventListener('provider-setup', (event: Event) => {
			const provider = (event as CustomEvent).detail;
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
		// Subscribe to state updates
		const unsubscribe = player.subscribe(({ paused, viewType }) => {
			// Handle state updates if needed
		});

		return () => {
			unsubscribe?.();
		};
	});

	function onProviderChange(event: MediaProviderChangeEvent) {
		const provider = event.detail;
		// Configure HLS provider
		if (isHLSProvider(provider)) {
			provider.config = {
				lowLatencyMode: true,
				manifestLoadingTimeOut: 10000,
				manifestLoadingMaxRetry: 3,
				maxBufferLength: 30,
				maxMaxBufferLength: 60,
				liveSyncDurationCount: 3,
				liveMaxLatencyDurationCount: 10,
				// Enable debug logs
				debug: false
			};
		}
	}

</script>

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
	<media-player
	  class="player"
	  title={name}
	  streamType="ll-live"
	  viewType="video"
	  src={videoUrl}
	  crossOrigin
	  playsInline
	  autoPlay
	  bind:this={player}
	>
		<media-provider>
			<media-poster
				class="vds-poster"
				src={xivi}
				alt={name}
			></media-poster>
		</media-provider>
		<!-- Layouts -->
		<media-video-layout>
			<media-controls>
				<media-controls-group class="absolute top-0 right-0 m-2">
					<media-cast-button></media-cast-button>
				</media-controls-group>
			</media-controls>
		</media-video-layout>
	</media-player>
	{/snippet}
</Modal>

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
