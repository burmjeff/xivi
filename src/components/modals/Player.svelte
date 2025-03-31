<!-- Player.svelte -->

<script lang="ts">
	// Import styles.
	import 'vidstack/player/styles/default/theme.css';
	import 'vidstack/player/styles/default/layouts/video.css';
	// Register elements.
	import 'vidstack/player';
	import 'vidstack/player/layouts';
	import 'vidstack/player/ui';
	import { isHLSProvider, MediaRemoteControl, type MediaProviderChangeEvent, type MediaErrorEvent } from 'vidstack';
	import type { MediaPlayerElement } from 'vidstack/elements';
	import { Modal } from '@skeletonlabs/skeleton-svelte';
	import { onMount } from 'svelte';
	import xivi from '@xivi/lib/assets/xivi.png';

	let { modalOpen = $bindable(), name, stream } = $props<{
		modalOpen: boolean;
		name: string;
		stream: string;
	}>();

	// Make videoUrl reactive to stream changes
	let videoUrl = $state(stream);
	
	// Update videoUrl when stream changes
	$effect(() => {
		videoUrl = stream;
		console.log('Stream URL updated:', videoUrl);
	});
	
	let player: HTMLElement | undefined = $state();
	const remote = new MediaRemoteControl();
	let isLoading = $state(true);
	let hasError = $state(false);
	let errorMessage = $state('');

	remote.disableCaptions();

	// Handle modal close - ensure player is properly cleaned up
	function handleModalClose() {
		if (player) {
			// Stop playback and reset player
			try {
				remote.pause();
				// Force player to unload current stream
				const mediaPlayer = player as MediaPlayerElement;
				mediaPlayer.src = '';
			} catch (e) {
				console.error('Error cleaning up player:', e);
			}
		}
		// Reset state
		isLoading = true;
		hasError = false;
		errorMessage = '';
	}

	// Handle player errors
	function onError(event: MediaErrorEvent) {
		console.error('Player error:', event.detail);
		isLoading = false;
		hasError = true;
		errorMessage = event.detail?.message || 'Failed to load stream';
	}

	// Handle player loaded state
	function onLoadedData() {
		console.log('Stream loaded successfully');
		isLoading = false;
	}

	onMount(() => {
		if (!player) return;

		// Connect the remote control to the player
		remote.setTarget(player as MediaPlayerElement);

		// Listen for provider setup events
		player.addEventListener('provider-change', onProviderChange as EventListener);
		player.addEventListener('error', onError as EventListener);
		player.addEventListener('loadeddata', onLoadedData as EventListener);
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
		const mediaPlayer = player as MediaPlayerElement;
		const unsubscribe = mediaPlayer.subscribe((state) => {
			// Update loading state based on state
			if (state.waiting || state.seeking) {
				isLoading = true;
			} else if (!isLoading && !hasError) {
				isLoading = false;
			}
		});

		return () => {
			// Clean up event listeners
			player?.removeEventListener('provider-change', onProviderChange as EventListener);
			player?.removeEventListener('error', onError as EventListener);
			player?.removeEventListener('loadeddata', onLoadedData as EventListener);
			unsubscribe?.();
		};
	});

	function onProviderChange(event: MediaProviderChangeEvent) {
		const provider = event.detail;
		console.log('Provider changed:', provider?.type);
		
		// Configure HLS provider with optimized settings
		if (isHLSProvider(provider)) {
			console.log('Configuring HLS provider for URL:', videoUrl);
			
			// Configure HLS provider
			provider.config = {
				// Use standard mode instead of low latency for better compatibility
				lowLatencyMode: false,
				// Increase timeouts and retries for better reliability
				manifestLoadingTimeOut: 30000,
				manifestLoadingMaxRetry: 5,
				// Optimize buffer settings for smoother playback
				maxBufferLength: 60,
				maxMaxBufferLength: 120,
				liveSyncDurationCount: 3,
				liveMaxLatencyDurationCount: 10,
				// Enable debug logs in development
				debug: true, // Enable debug logs to help troubleshoot
				// Additional settings for better performance
				startLevel: -1, // Auto-select quality level
				abrEwmaDefaultEstimate: 500000, // 500kbps default bandwidth estimate
				abrBandWidthFactor: 0.95, // Conservative bandwidth usage
				abrBandWidthUpFactor: 0.7, // Conservative bandwidth upgrade
				// Improve stream switching
				testBandwidth: true,
				// Increase fragment loading timeout
				fragLoadingTimeOut: 20000,
				xhrSetup: (xhr: XMLHttpRequest) => {
					// Set additional headers if needed
					xhr.withCredentials = false;
					// Log XHR requests for debugging
					console.log('HLS XHR request:', xhr.responseURL || 'unknown URL');
				}
			};
			
			// Force player to reload the source
			if (player) {
				const mediaPlayer = player as MediaPlayerElement;
				// Reset the source to force reload
				setTimeout(() => {
					mediaPlayer.src = videoUrl;
					remote.startLoading();
					remote.play();
				}, 100);
			}
		}
	}

	// Watch for modal open/close
	$effect(() => {
		if (!modalOpen) {
			handleModalClose();
		} else {
			// Modal opened - reset loading state
			isLoading = true;
			hasError = false;
			errorMessage = '';
			
			// If player exists, force reload the stream
			if (player) {
				const mediaPlayer = player as MediaPlayerElement;
				// Small delay to ensure DOM is ready
				setTimeout(() => {
					console.log('Reloading player with URL:', videoUrl);
					
					// Log network requests for debugging
					console.log('Checking network connectivity to stream URL...');
					fetch(videoUrl, { method: 'HEAD' })
						.then(response => {
							console.log('Stream URL response:', response.status, response.statusText);
							console.log('Stream URL headers:', response.headers);
							
							// Set source and play
							mediaPlayer.src = videoUrl;
							remote.play();
						})
						.catch(error => {
							console.error('Error fetching stream URL:', error);
							isLoading = false;
							hasError = true;
							errorMessage = `Network error: ${error.message}. Check browser console for details.`;
						});
				}, 300); // Increased delay for better reliability
			}
		}
	});
</script>

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
	<div class="player-container relative">
		<!-- Loading indicator -->
		{#if isLoading}
		<div class="absolute inset-0 z-10 flex items-center justify-center bg-black/50">
			<div class="flex items-center justify-center">
				<div class="spinner-border animate-spin inline-block w-8 h-8 border-4 rounded-full border-primary-500 border-t-transparent" role="status"></div>
				<span class="ml-2 text-white">Loading stream...</span>
			</div>
		</div>
		{/if}

		<!-- Error message -->
		{#if hasError}
		<div class="absolute inset-0 z-10 flex flex-col items-center justify-center bg-black/80 p-4 text-center">
			<span class="text-xl text-error-500">Stream Error</span>
			<p class="mt-2 text-white">{errorMessage}</p>
			<button class="btn btn-sm variant-filled-primary mt-4" onclick={() => {
				hasError = false;
				isLoading = true;
				
				// Force reload the stream
				if (player) {
					const mediaPlayer = player as MediaPlayerElement;
					mediaPlayer.src = videoUrl;
					setTimeout(() => remote.play(), 500);
				}
			}}>
				Try Again
			</button>
		</div>
		{/if}

		<!-- Media player -->
		<media-player
		  class="player"
		  title={name}
		  streamType="live"
		  viewType="video"
		  src={videoUrl}
		  crossOrigin="anonymous"
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
			<!-- Enhanced layout with more controls -->
			<media-video-layout>
				<!-- Top controls -->
				<media-controls class="media-controls">
					<media-controls-group class="absolute top-0 right-0 m-2">
						<media-cast-button></media-cast-button>
						<media-fullscreen-button></media-fullscreen-button>
					</media-controls-group>

					<!-- Bottom controls -->
					<media-controls-group class="absolute bottom-0 left-0 right-0 flex w-full items-center justify-between p-2">
						<div class="flex items-center gap-2">
							<media-play-button></media-play-button>
							<media-mute-button></media-mute-button>
							<media-volume-slider></media-volume-slider>
						</div>
						<div class="flex items-center gap-2">
							<media-time-display></media-time-display>
						</div>
					</media-controls-group>
				</media-controls>
			</media-video-layout>
		</media-player>
	</div>
	{/snippet}
</Modal>

<style lang="postcss">
	.player-container {
		position: relative;
		width: 100%;
		height: 100%;
		overflow: hidden;
		background-color: black;
		border-radius: 4px;
	}

	.player {
		max-height: 100vh;
		max-width: 100%;
		height: 57vh;
		width: 100%;
		aspect-ratio: 16 /9;
		--brand-color: #f5f5f5;
		--focus-color: #4e9cf6;

		--video-brand: var(--brand-color);
		--video-focus-ring-color: var(--focus-color);
		--video-border-radius: 4px;
		--media-control-background: rgba(0, 0, 0, 0.6);
		--media-control-hover-background: rgba(0, 0, 0, 0.8);
		--media-control-border-radius: 4px;

		/* Improve visibility of controls */
		--media-button-icon-size: 24px;
		--media-button-size: 36px;
		--media-time-font-size: 14px;
		--media-time-font-weight: 500;
		--media-time-color: white;
	}

	/* Ensure controls are visible on hover */
	.media-controls {
		opacity: 0;
		transition: opacity 0.2s ease-in-out;
	}

	.player:hover .media-controls {
		opacity: 1;
	}

	/* Ensure poster image covers the player area */
	.vds-poster {
		object-fit: cover;
		width: 100%;
		height: 100%;
	}

	/* Custom spinner animation */
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.animate-spin {
		animation: spin 1s linear infinite;
	}
</style>
