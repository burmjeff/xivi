<!-- Player.svelte -->

<script lang="ts">
	import 'media-chrome';
	import Hls from 'hls.js';
	import { Modal } from '@skeletonlabs/skeleton-svelte';
	import { onMount } from 'svelte';
	import xivi from '@xivi/lib/assets/xivi.png';

	// Component props
	let {
		modalOpen = $bindable(),
		name,
		stream
	} = $props<{
		modalOpen: boolean;
		name: string;
		stream: string;
	}>();

	// Component state
	let videoUrl = $state(stream);
	let videoElement: HTMLVideoElement | undefined = $state();
	let hlsInstance: Hls | undefined = $state();
	let isLoading = $state(true);
	let hasError = $state(false);
	let errorMessage = $state('');
	let setupInProgress = $state(false);

	// Update stream URL when prop changes
	$effect(() => {
		if (stream !== videoUrl) {
			videoUrl = stream;

			if (videoElement && videoUrl) {
				cleanupPlayer();
				setTimeout(() => setupHls(), 300);
			}
		}
	});

	// Handle modal open/close
	$effect(() => {
		if (!modalOpen) {
			cleanupPlayer();
		} else if (videoElement && videoUrl && !hlsInstance) {
			setTimeout(() => setupHls(), 300);
		}
	});

	// Watch for video element availability
	$effect(() => {
		if (videoElement && modalOpen && videoUrl && !hlsInstance && !setupInProgress) {
			setTimeout(() => setupHls(), 100);
		}
	});

	// Clean up player resources
	function cleanupPlayer() {
		// Prevent cleanup if already in progress
		if (setupInProgress) {
			setupInProgress = false;
		}

		// Clean up HLS instance
		if (hlsInstance) {
			try {
				hlsInstance.stopLoad();
				hlsInstance.detachMedia();
				hlsInstance.destroy();
			} catch (e) {
				console.error('Error cleaning up HLS instance:', e);
			}
			hlsInstance = undefined;
		}

		// Clean up video element
		if (videoElement) {
			try {
				videoElement.pause();
				videoElement.src = '';
				videoElement.load();
			} catch (error) {
				console.error('Error cleaning up video element:', error);
			}
		}

		// Reset state
		isLoading = true;
		hasError = false;
		errorMessage = '';
	}

	// Setup HLS player
	function setupHls() {
		if (!videoElement || !videoUrl || setupInProgress) return;

		setupInProgress = true;
		isLoading = true;
		hasError = false;
		errorMessage = '';

		// Ensure video is visible
		if (videoElement) {
			videoElement.style.display = 'block';
		}

		// Check if HLS.js is supported
		if (Hls.isSupported()) {
			// Create new HLS instance
			hlsInstance = new Hls({
				// Basic settings
				lowLatencyMode: true,
				autoStartLoad: true, // Auto start loading
				startLevel: -1, // Auto-select quality level
				enableWorker: true, // Use web workers

				// Manifest loading settings
				manifestLoadingTimeOut: 5000,
				manifestLoadingMaxRetry: 5,
				manifestLoadingRetryDelay: 500,
				manifestLoadingMaxRetryTimeout: 5000, // Cap retry delay

				// Fragment loading settings
				fragLoadingTimeOut: 5000,
				fragLoadingMaxRetry: 5,
				fragLoadingRetryDelay: 500,
				fragLoadingMaxRetryTimeout: 5000, // Cap retry delay
				startFragPrefetch: true, // Prefetch fragments

				// Buffer settings
				maxBufferLength: 30, // Buffer length in seconds
				maxMaxBufferLength: 60, // Maximum buffer length
				maxBufferHole: 0.1, // Max buffer hole in seconds
				highBufferWatchdogPeriod: 1, // High buffer watchdog period
				abrEwmaDefaultEstimate: 500000, // Default estimate for ABR
				abrBandWidthFactor: 0.95, // Bandwidth factor for ABR
				abrBandWidthUpFactor: 0.7, // Bandwidth up factor for ABR

				// Performance settings
				testBandwidth: true, // Test bandwidth for ABR
				progressive: true, // Enable progressive loading
				appendErrorMaxRetry: 5, // Max retries for append errors

				// Live stream settings
				liveBackBufferLength: 30, // Live back buffer length
				liveSyncDurationCount: 2, // Number of segments to sync with live
				liveMaxLatencyDurationCount: 10, // Max latency duration count

				// Segment transition settings
				maxFragLookUpTolerance: 0.15, // Fragment lookup tolerance
				maxStarvationDelay: 1, // Max starvation delay
				maxLoadingDelay: 1 // Max loading delay
			});

			// Bind HLS to video element
			hlsInstance.attachMedia(videoElement);

			// Load source
			hlsInstance.on(Hls.Events.MEDIA_ATTACHED, () => {
				hlsInstance?.loadSource(videoUrl);
			});

			// Handle manifest parsed - ready to play
			hlsInstance.on(Hls.Events.MANIFEST_PARSED, () => {
				isLoading = false;
				setupInProgress = false;
				videoElement?.play().catch((error) => {
					console.error('Error auto-playing video:', error);
				});
			});

			// Handle level loading to prefetch segments
			hlsInstance.on(Hls.Events.LEVEL_LOADED, (_, data) => {
				// Monitor level loading for debugging
				if (data.details.live && data.details.targetduration) {
					console.debug(`HLS level loaded: target duration ${data.details.targetduration}s`);
				}
			});

			// Handle fragment loading
			hlsInstance.on(Hls.Events.FRAG_LOADING, (_, data) => {
				console.debug(`Loading fragment: ${data.frag.sn} of level ${data.frag.level}`);
			});

			// Handle fragment buffered event
			hlsInstance.on(Hls.Events.FRAG_BUFFERED, (_, data) => {
				console.debug(`Fragment buffered: ${data.frag.sn} (${data.stats.total}ms)`);
				if (videoElement && !videoElement.paused) {
					const currentTime = videoElement.currentTime;
					const buffered = videoElement.buffered;

					// Check if we need to force buffer update
					if (buffered.length > 0) {
						const bufferEnd = buffered.end(buffered.length - 1);
						if (bufferEnd - currentTime < 2) {
							hlsInstance?.startLoad(); // Force reload if buffer is low
						}
					}
				}
			});

			// Handle buffer appended
			hlsInstance.on(Hls.Events.BUFFER_APPENDED, () => {
				// Buffer appended, ensure playback continues
				if (videoElement && videoElement.paused && !isLoading) {
					videoElement.play().catch((e) => console.debug('Auto-resume failed:', e));
				}
			});

			// Handle buffer EOS
			hlsInstance.on(Hls.Events.BUFFER_EOS, () => {
				console.debug('Buffer EOS reached');
			});

			// Handle errors
			hlsInstance.on(Hls.Events.ERROR, (_, data) => {
				if (data.fatal) {
					switch (data.type) {
						case Hls.ErrorTypes.NETWORK_ERROR:
							hlsInstance?.startLoad();
							break;
						case Hls.ErrorTypes.MEDIA_ERROR:
							hlsInstance?.recoverMediaError();
							break;
						default:
							cleanupPlayer();
							hasError = true;
							isLoading = false;
							setupInProgress = false;
							errorMessage = `HLS Error: ${data.details}`;
							break;
					}
				}
			});
		} else if (videoElement.canPlayType('application/vnd.apple.mpegurl')) {
			// For Safari which has native HLS support
			videoElement.src = videoUrl;
			videoElement.addEventListener('loadedmetadata', () => {
				isLoading = false;
				setupInProgress = false;
				videoElement?.play().catch((error) => {
					console.error('Error auto-playing video:', error);
				});
			});

			// Handle errors
			videoElement.addEventListener('error', () => {
				hasError = true;
				isLoading = false;
				setupInProgress = false;
				errorMessage = `Video Error: ${videoElement?.error?.message || 'Unknown error'}`;
			});
		} else {
			// HLS not supported
			hasError = true;
			isLoading = false;
			setupInProgress = false;
			errorMessage = 'HLS playback is not supported in this browser';
		}
	}

	// Handle retry button click
	function handleRetry() {
		cleanupPlayer();
		setTimeout(() => setupHls(), 1000);
	}

	// Clean up on component unmount
	onMount(() => {
		return () => {
			cleanupPlayer();
		};
	});
</script>

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	backdropClasses="backdrop-blur-sm"
	contentBase="card p-0 shadow-xl"
>
	{#snippet content()}
		<!-- Loading indicator -->
		{#if isLoading}
			<div class="absolute inset-0 flex items-center justify-center bg-black/50">
				<div class="flex items-center justify-center">
					<div
						class="spinner-border border-primary-500 inline-block h-8 w-8 animate-spin rounded-full border-4 border-t-transparent"
						role="status"
					></div>
					<span class="ml-2 text-white">Loading stream...</span>
				</div>
			</div>
		{/if}

		<!-- Error message -->
		{#if hasError}
			<div
				class="absolute inset-0 flex flex-col items-center justify-center bg-black/80 p-4 text-center"
			>
				<span class="text-error-500 text-xl">Stream Error</span>
				<p class="mt-2 text-white">{errorMessage}</p>
				<button class="btn btn-sm variant-filled-primary mt-4" onclick={handleRetry}>
					Retry
				</button>
			</div>
		{/if}

		<!-- Media Chrome player -->
		<media-controller>
			<video
				bind:this={videoElement}
				slot="media"
				autoplay
				muted={false}
				poster={xivi}
				title={name || 'Video Stream'}
				playsinline
			></video>

			<!-- Media Chrome UI -->
			<media-control-bar>
				<media-play-button></media-play-button>
				<media-seek-backward-button></media-seek-backward-button>
				<media-seek-forward-button></media-seek-forward-button>
				<media-mute-button></media-mute-button>
				<media-volume-range></media-volume-range>
				<media-time-range></media-time-range>
				<media-time-display showduration remaining></media-time-display>
				<media-playback-rate-button></media-playback-rate-button>
				<media-fullscreen-button></media-fullscreen-button>
			</media-control-bar>
		</media-controller>
	{/snippet}
</Modal>

<style>
	:global([data-scope='dialog'][data-part='content']) {
		max-width: 75vw !important;
		max-height: 75vh !important;
	}
</style>
