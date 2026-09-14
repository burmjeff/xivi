<script lang="ts">
	import 'media-chrome';
	import Hls from 'hls.js';
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onDestroy, onMount, untrack } from 'svelte';
	import { ChevronDown, ChevronLeft, ChevronRight, RotateCcw, Radio, Play } from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type {
		GuideChannel,
		LineupSummary,
		Paginated,
		WatchChannelNeighbors
	} from '$lib/api/types';
	import { preferences, selectLineup } from '$lib/state/preferences.svelte';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import { playbackController } from '$lib/playback/controller';
	import { XiviNative, type NativePlaybackState } from '$lib/platform/native';
	import NativeVideoSurface from './NativeVideoSurface.svelte';

	let routeMatch = $derived(page.url.pathname.match(/^\/watch\/channel\/(\d+)\/?$/));
	let channelId = $derived(routeMatch ? Number(routeMatch[1]) : 0);
	let playerRoute = $derived(channelId > 0);
	const lineupsQuery = createQuery(() => ({
		queryKey: ['watch', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/watch/lineups')
	}));
	let requestedLineupId = $derived(Number(page.url.searchParams.get('lineup')) || null);
	let lineupId = $derived(
		requestedLineupId ?? preferences.lineupId ?? lineupsQuery.data?.items[0]?.id ?? null
	);
	$effect(() => {
		if (lineupId && preferences.lineupId !== lineupId) selectLineup(lineupId);
	});
	const channelQuery = createQuery(() => ({
		queryKey: ['watch', 'channel', lineupId, channelId],
		enabled: playerRoute && !!lineupId,
		queryFn: () => api<GuideChannel>(`/api/v2/watch/channels/${channelId}?lineup_id=${lineupId}`),
		refetchInterval: 60_000
	}));
	const neighborsQuery = createQuery(() => ({
		queryKey: ['watch', 'channel-neighbors', lineupId, channelId],
		enabled: playerRoute && !!lineupId,
		queryFn: () =>
			api<WatchChannelNeighbors>(
				`/api/v2/watch/lineups/${lineupId}/channels/${channelId}/neighbors`
			)
	}));
	let video = $state<HTMLVideoElement>(),
		hls: Hls | null = null,
		activeChannel = $state<GuideChannel | null>(null),
		loading = $state(true),
		error = $state(''),
		playbackPaused = $state(true),
		attachedUrl = '',
		networkRetries = 0,
		mediaRecoveries = 0,
		hlsRebuilds = 0,
		attachmentGeneration = 0,
		hlsRebuilding = false,
		playbackReady = $state(false),
		viewerId = '',
		incidentId = $state(''),
		lastTelemetryAt = 0,
		retryTimer: ReturnType<typeof setTimeout> | undefined,
		stallTimer: ReturnType<typeof setTimeout> | undefined;
	let pipActive = $state(false);
	let nativeState = $state<NativePlaybackState>({
		active: false,
		playing: false
	});
	let nativeRequestedStream = '';
	let nativeStateReady = $state(!playbackController.native);
	let nativeChannelActive = $derived(
		nativeState.active &&
			nativeState.channelId === activeChannel?.id &&
			nativeState.lineupId === lineupId
	);
	let removeNativeListener: (() => void) | undefined;
	let removeOpenListener: (() => void) | undefined;
	let destroyed = false;
	onMount(() => {
		const minimize = () => {
			if (playerRoute) void goto('/channels');
		};
		window.addEventListener('xiviMinimizePlayer', minimize);
		return () => window.removeEventListener('xiviMinimizePlayer', minimize);
	});
	if (playbackController.native) {
		let receivedState = false;
		void playbackController
			.state()
			.then((state) => {
				if (!receivedState) nativeState = state;
				nativeStateReady = true;
			})
			.catch(() => {
				nativeStateReady = true;
			});
		void playbackController
			.subscribe((state) => {
				receivedState = true;
				nativeState = state;
				nativeStateReady = true;
			})
			.then((remove) => {
				if (destroyed) remove();
				else removeNativeListener = remove;
			});
		void XiviNative.addListener('openPlayer', (item) => {
			void goto(`/watch/channel/${item.channelId}?lineup=${item.lineupId}`);
		}).then((handle) => {
			if (destroyed) void handle.remove();
			else removeOpenListener = () => void handle.remove();
		});
	}
	function ensureViewerId() {
		if (!viewerId) {
			viewerId =
				globalThis.crypto?.randomUUID?.() ??
				`viewer-${Date.now()}-${Math.random().toString(36).slice(2)}`;
		}
		return viewerId;
	}
	function streamId(url: string) {
		try {
			const parts = new URL(url, location.origin).pathname.split('/').filter(Boolean);
			return parts.at(-1) ?? '';
		} catch {
			return '';
		}
	}
	function viewerUrl(url: string) {
		const result = new URL(url, location.origin);
		result.searchParams.set('viewer_id', ensureViewerId());
		return `${result.pathname}${result.search}${result.hash}`;
	}
	async function reportPlayerEvent(
		url: string,
		severity: 'info' | 'warning' | 'error',
		code: string,
		message: string,
		details: Record<string, unknown> = {},
		throttle = false
	) {
		if (throttle && Date.now() - lastTelemetryAt < 30_000) return;
		lastTelemetryAt = Date.now();
		try {
			const result = await api<{ incident_id?: string }>('/api/v2/stream/telemetry', {
				method: 'POST',
				body: JSON.stringify({
					stream_id: streamId(url),
					viewer_id: ensureViewerId(),
					severity,
					code,
					message,
					details: {
						...details,
						ready_state: video?.readyState,
						network_state: video?.networkState,
						buffered_seconds:
							video && video.buffered.length
								? Math.max(0, video.buffered.end(video.buffered.length - 1) - video.currentTime)
								: 0
					}
				})
			});
			if (result.incident_id) incidentId = result.incident_id;
		} catch {
			// Playback diagnostics must never interfere with recovery.
		}
	}
	let previous = $derived(neighborsQuery.data?.previous);
	let next = $derived(neighborsQuery.data?.next);
	let upcoming = $derived.by(() => {
		const channel = activeChannel;
		if (!channel) return [];
		const now = Date.now();
		return [
			...new Map(
				[...(channel.next ? [channel.next] : []), ...(channel.programmes ?? [])]
					.filter(
						(programme) =>
							programme.id !== channel.current?.id && new Date(programme.start).getTime() > now
					)
					.map((programme) => [programme.id, programme])
			).values()
		]
			.sort((a, b) => new Date(a.start).getTime() - new Date(b.start).getTime())
			.slice(0, 5);
	});
	const prewarmed = new Set<string>();
	$effect(() => {
		if (!playbackReady || !playerRoute) return;
		const candidates = [previous, next];
		const timers: ReturnType<typeof setTimeout>[] = [];
		for (const [index, candidate] of candidates.entries()) {
			const id = candidate?.stream_url ? streamId(candidate.stream_url) : '';
			if (!id || prewarmed.has(id)) continue;
			timers.push(
				setTimeout(
					() => {
						if (!playbackReady || prewarmed.has(id)) return;
						prewarmed.add(id);
						void api(`/api/v2/stream/prewarm/${id}`, { method: 'POST' }).catch(() => {
							// Prewarming is opportunistic and may be refused by a source connection budget.
						});
					},
					750 + index * 1_000
				)
			);
		}
		return () => timers.forEach(clearTimeout);
	});

	function cleanup() {
		attachmentGeneration += 1;
		hlsRebuilding = false;
		if (retryTimer) clearTimeout(retryTimer);
		if (stallTimer) clearTimeout(stallTimer);
		retryTimer = undefined;
		stallTimer = undefined;
		if (hls) {
			hls.destroy();
			hls = null;
		}
		if (video) {
			video.pause();
			video.removeAttribute('src');
			video.load();
		}
		attachedUrl = '';
		playbackReady = false;
	}
	function releasePlayback() {
		const id = attachedUrl ? streamId(attachedUrl) : '';
		if (!id || !viewerId) return;
		void api('/api/v2/watch/playback/release', {
			method: 'POST',
			body: JSON.stringify({ playback_id: viewerId, stream_id: id }),
			keepalive: true
		}).catch(() => {
			// Socket closure and server lease expiry remain the fallback.
		});
	}
	function stopPlayback() {
		releasePlayback();
		cleanup();
		activeChannel = null;
		pipActive = false;
	}
	function onEnterPictureInPicture() {
		pipActive = true;
	}
	function onLeavePictureInPicture() {
		pipActive = false;
		if (!playerRoute) stopPlayback();
	}
	function pictureInPictureEvents(node: HTMLVideoElement) {
		node.addEventListener('enterpictureinpicture', onEnterPictureInPicture);
		node.addEventListener('leavepictureinpicture', onLeavePictureInPicture);
		return {
			destroy() {
				node.removeEventListener('enterpictureinpicture', onEnterPictureInPicture);
				node.removeEventListener('leavepictureinpicture', onLeavePictureInPicture);
			}
		};
	}
	function onPlaying() {
		loading = false;
		error = '';
		playbackPaused = false;
		networkRetries = 0;
		mediaRecoveries = 0;
		hlsRebuilds = 0;
		playbackReady = true;
		if (stallTimer) clearTimeout(stallTimer);
	}
	function onCanPlay() {
		loading = false;
		if (stallTimer) clearTimeout(stallTimer);
	}
	function onWaiting() {
		loading = true;
		if (stallTimer) clearTimeout(stallTimer);
		stallTimer = setTimeout(() => {
			if (!video || video.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA) return;
			if (hls && !hls.loadingEnabled) hls.startLoad(-1);
			const liveSyncPosition = hls?.liveSyncPosition;
			if (liveSyncPosition != null && liveSyncPosition - video.currentTime > 12) {
				video.currentTime = liveSyncPosition;
			}
			if (attachedUrl)
				void reportPlayerEvent(
					attachedUrl,
					'warning',
					'player_buffer_stall',
					'The browser ran out of playable media.',
					{},
					true
				);
		}, 4_000);
	}
	async function startPlayback() {
		if (!video) return;
		try {
			await video.play();
			playbackPaused = video.paused;
		} catch (playError) {
			if (playError instanceof DOMException && playError.name === 'NotAllowedError') {
				loading = false;
				playbackPaused = true;
				return;
			}
			if (playError instanceof DOMException && playError.name === 'AbortError') return;
			loading = false;
			error = 'The browser could not start this live stream. Retry to reconnect.';
			if (attachedUrl)
				void reportPlayerEvent(attachedUrl, 'error', 'player_start_failed', error, {
					name: playError instanceof Error ? playError.name : 'unknown'
				});
		}
	}
	function scheduleNetworkRecovery(url: string) {
		if (!hls || attachedUrl !== url) return;
		if (networkRetries >= 5) {
			loading = false;
			error = 'The source stayed unavailable after several reconnect attempts.';
			void reportPlayerEvent(url, 'error', 'player_network_retries_exhausted', error, {
				retries: networkRetries
			});
			return;
		}
		networkRetries += 1;
		const delay = Math.min(8_000, 500 * 2 ** (networkRetries - 1));
		loading = true;
		error = `Signal interrupted. Reconnecting (${networkRetries}/5)…`;
		hls.stopLoad();
		if (retryTimer) clearTimeout(retryTimer);
		retryTimer = setTimeout(() => {
			if (hls && attachedUrl === url) hls.startLoad(-1);
		}, delay);
	}
	function isMissingSourceBufferError(data: { details?: string; error?: unknown }) {
		const message = data.error instanceof Error ? data.error.message : '';
		return (
			data.details === Hls.ErrorDetails.BUFFER_APPEND_ERROR &&
			/SourceBuffer.*does not exist|append to the .*SourceBuffer/i.test(message)
		);
	}
	function rebuildHlsInstance(
		url: string,
		instance: Hls,
		generation: number,
		details: Record<string, unknown>
	) {
		if (hls !== instance || attachmentGeneration !== generation || hlsRebuilds >= 2) return false;
		hlsRebuilds += 1;
		loading = true;
		error = 'The player is rebuilding the live signal…';
		// recoverMediaError() retains the existing SourceBuffer topology. When a
		// startup fragment omitted video, only a fresh MediaSource/Hls instance
		// can create both buffers from the corrected playlist.
		attachmentGeneration += 1;
		hlsRebuilding = true;
		hls = null;
		instance.destroy();
		if (video) {
			video.pause();
			video.removeAttribute('src');
			video.load();
		}
		void reportPlayerEvent(
			url,
			'warning',
			'hls_source_buffer_rebuild',
			'HLS.js is rebuilding a missing media SourceBuffer.',
			{ ...details, rebuild: hlsRebuilds }
		);
		if (retryTimer) clearTimeout(retryTimer);
		retryTimer = setTimeout(() => {
			retryTimer = undefined;
			if (attachedUrl === url && (playerRoute || pipActive)) {
				void attach(url, { force: true, recovery: true });
			}
		}, 300 * hlsRebuilds);
		return true;
	}
	function onVideoError() {
		const details = {
			media_error_code: video?.error?.code,
			media_error_message: video?.error?.message
		};
		if (hls || hlsRebuilding) {
			if (attachedUrl)
				void reportPlayerEvent(
					attachedUrl,
					'warning',
					'native_media_error_during_hls',
					'The browser media element reported an HLS playback error.',
					details,
					true
				);
			return;
		}
		loading = false;
		error = 'The native player lost the live signal. Retry to reconnect.';
		if (attachedUrl)
			void reportPlayerEvent(attachedUrl, 'error', 'native_media_error', error, details);
	}
	async function attach(url: string, options: { force?: boolean; recovery?: boolean } = {}) {
		if (!video || !url || (!options.force && attachedUrl === url)) return;
		cleanup();
		if (!options.recovery) {
			networkRetries = 0;
			mediaRecoveries = 0;
			hlsRebuilds = 0;
		}
		attachedUrl = url;
		incidentId = '';
		loading = true;
		error = '';
		playbackPaused = true;
		playbackReady = false;
		if (Hls.isSupported()) {
			const generation = attachmentGeneration;
			const instance = new Hls({
				enableWorker: true,
				lowLatencyMode: false,
				initialLiveManifestSize: 1,
				startFragPrefetch: true,
				backBufferLength: 15,
				maxBufferLength: 20,
				maxMaxBufferLength: 30,
				maxBufferHole: 0.5,
				highBufferWatchdogPeriod: 4,
				nudgeOffset: 0.15,
				nudgeMaxRetry: 5,
				liveSyncDurationCount: 2,
				liveSyncOnStallIncrease: 1,
				liveMaxLatencyDurationCount: 6,
				maxLiveSyncPlaybackRate: 1.05,
				manifestLoadPolicy: {
					default: {
						maxTimeToFirstByteMs: 8_000,
						maxLoadTimeMs: 15_000,
						timeoutRetry: {
							maxNumRetry: 2,
							retryDelayMs: 250,
							maxRetryDelayMs: 2_000,
							backoff: 'exponential'
						},
						errorRetry: {
							maxNumRetry: 3,
							retryDelayMs: 500,
							maxRetryDelayMs: 4_000,
							backoff: 'exponential'
						}
					}
				},
				playlistLoadPolicy: {
					default: {
						maxTimeToFirstByteMs: 8_000,
						maxLoadTimeMs: 15_000,
						timeoutRetry: {
							maxNumRetry: 3,
							retryDelayMs: 250,
							maxRetryDelayMs: 2_000,
							backoff: 'exponential'
						},
						errorRetry: {
							maxNumRetry: 5,
							retryDelayMs: 500,
							maxRetryDelayMs: 4_000,
							backoff: 'exponential'
						}
					}
				},
				fragLoadPolicy: {
					default: {
						maxTimeToFirstByteMs: 8_000,
						maxLoadTimeMs: 30_000,
						timeoutRetry: {
							maxNumRetry: 4,
							retryDelayMs: 250,
							maxRetryDelayMs: 2_000,
							backoff: 'exponential'
						},
						errorRetry: {
							maxNumRetry: 6,
							retryDelayMs: 500,
							maxRetryDelayMs: 4_000,
							backoff: 'exponential'
						}
					}
				}
			});
			hlsRebuilding = false;
			hls = instance;
			instance.on(Hls.Events.MANIFEST_PARSED, () => {
				if (hls !== instance || attachmentGeneration !== generation) return;
				void startPlayback();
			});
			instance.on(Hls.Events.ERROR, (_event, data) => {
				if (hls !== instance || attachmentGeneration !== generation) return;
				const hlsDetails = {
					type: data.type,
					detail: data.details,
					fatal: data.fatal,
					mime_type: 'mimeType' in data ? String(data.mimeType ?? '') : undefined,
					source_buffer:
						'sourceBufferName' in data ? String(data.sourceBufferName ?? '') : undefined,
					reason: 'reason' in data ? String(data.reason ?? '') : undefined,
					media_error:
						'error' in data && data.error instanceof Error ? data.error.message : undefined
				};
				if (!data.fatal) {
					if (data.details === Hls.ErrorDetails.BUFFER_ADD_CODEC_ERROR) {
						void reportPlayerEvent(
							url,
							'warning',
							'hls_codec_rejected',
							'The browser rejected an HLS media codec.',
							hlsDetails
						);
					}
					if (data.details === Hls.ErrorDetails.BUFFER_STALLED_ERROR) {
						onWaiting();
						void reportPlayerEvent(
							url,
							'warning',
							'hls_buffer_stalled',
							'HLS.js reported a playback stall.',
							hlsDetails,
							true
						);
					}
					return;
				}
				if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
					scheduleNetworkRecovery(url);
					return;
				}
				if (
					data.type === Hls.ErrorTypes.MEDIA_ERROR &&
					isMissingSourceBufferError(data) &&
					rebuildHlsInstance(url, instance, generation, hlsDetails)
				) {
					return;
				}
				if (data.type === Hls.ErrorTypes.MEDIA_ERROR && mediaRecoveries < 2) {
					mediaRecoveries += 1;
					loading = true;
					error = 'The player is repairing the live signal…';
					if (mediaRecoveries === 2) instance.swapAudioCodec();
					instance.recoverMediaError();
					void reportPlayerEvent(
						url,
						'warning',
						'hls_media_recovery',
						'HLS.js is attempting media recovery.',
						{ ...hlsDetails, recovery: mediaRecoveries }
					);
					return;
				}
				loading = false;
				error = 'This stream could not be decoded by the browser.';
				void reportPlayerEvent(url, 'error', 'hls_fatal_error', error, hlsDetails);
			});
			instance.attachMedia(video);
			instance.loadSource(viewerUrl(url));
		} else if (video.canPlayType('application/vnd.apple.mpegurl')) {
			video.src = viewerUrl(url);
			video.load();
			void startPlayback();
		} else {
			loading = false;
			error = 'HLS playback is not supported in this browser.';
			void reportPlayerEvent(url, 'error', 'hls_unsupported', error);
		}
	}
	async function openNativePlayer(reuseSession = true) {
		if (!activeChannel || !lineupId) return;
		error = '';
		try {
			if (reuseSession && nativeChannelActive && !nativeState.error) {
				await playbackController.reopen();
			} else {
				await playbackController.playVideo({
					channelId: activeChannel.id,
					lineupId,
					name: activeChannel.name,
					programme: activeChannel.current?.title,
					logoUrl: activeChannel.logo_url,
					streamUrl: activeChannel.stream_url
				});
			}
		} catch (playError) {
			error =
				playError instanceof Error ? playError.message : 'The player could not start this channel.';
		}
	}
	async function stopNativePlayback() {
		try {
			await playbackController.stop();
			error = '';
		} catch {
			error = 'The player could not stop. Please try again.';
		}
	}
	async function returnToNativeChannel() {
		if (nativeState.channelId && nativeState.lineupId) {
			await goto(`/watch/channel/${nativeState.channelId}?lineup=${nativeState.lineupId}`);
		} else {
			await playbackController.reopen();
		}
	}
	$effect(() => {
		if (!playerRoute || !channelQuery.data) return;
		activeChannel = channelQuery.data;
		if (playbackController.native && lineupId) {
			if (!nativeStateReady) return;
			loading = false;
			error = '';
			const key = `${lineupId}:${channelQuery.data.id}:${channelQuery.data.stream_url}`;
			if (nativeRequestedStream === key) return;
			nativeRequestedStream = key;
			// Returning from PiP or the notification must keep the current stream and position.
			untrack(() => {
				if (
					nativeState.active &&
					nativeState.channelId === channelQuery.data?.id &&
					nativeState.lineupId === lineupId
				)
					return;
				void openNativePlayer(false);
			});
		} else if (video && channelQuery.data.stream_url) void attach(channelQuery.data.stream_url);
	});
	$effect(() => {
		if (playbackController.native || playerRoute || !attachedUrl || pipActive) return;
		// Internal navigation updates the root shell without unmounting this
		// component. Stop ordinary background playback, but leave an active PiP
		// session and its exact video element untouched.
		stopPlayback();
	});
	onDestroy(() => {
		destroyed = true;
		removeNativeListener?.();
		removeOpenListener?.();
		if (!playbackController.native) stopPlayback();
	});
	function programmeTime(value?: string) {
		return value
			? new Intl.DateTimeFormat([], { hour: 'numeric', minute: '2-digit' }).format(new Date(value))
			: '';
	}
</script>

<svelte:head>
	{#if playerRoute && activeChannel}<title>{activeChannel.name} · Xivi</title>{/if}
</svelte:head>
<svelte:window onpagehide={releasePlayback} />
<section
	class="player-page"
	class:native={playbackController.native}
	class:pip-background={!playerRoute}
	aria-hidden={!playerRoute}
>
	<div class="player-dock">
		<div class="player-bar">
			<button
				type="button"
				aria-label="Minimize player"
				title="Minimize player"
				onclick={() =>
					playbackController.native
						? goto('/channels')
						: history.length > 1
							? history.back()
							: goto('/')}><ChevronDown size={20} aria-hidden="true" /><span>Minimize</span></button
			>
			<div class="player-bar-actions">
				{#if playbackController.native && nativeChannelActive}<button
						onclick={() => void stopNativePlayback()}>Stop</button
					>{/if}
				<span class="live-pill">Live</span>
			</div>
		</div>
		<div class="player-stage">
			{#if playbackController.native}
				{#if nativeChannelActive && playerRoute && !error}
					<NativeVideoSurface mode="inline" />
				{:else}
					<div class="native-player-launch">
						<h2>Watch live</h2>
						<p>
							{error ||
								(nativeChannelActive && nativeState.error) ||
								(nativeChannelActive
									? nativeState.loading
										? 'Connecting to the channel…'
										: 'The live signal is ready.'
									: 'Watch this channel here.')}
						</p>
						<div>
							<button
								class="app-button app-button--primary"
								disabled={!activeChannel || !lineupId}
								onclick={() => void openNativePlayer()}
							>
								Start watching</button
							>
						</div>
					</div>
				{/if}
			{:else}
				<media-controller class="media-controller">
					<!-- svelte-ignore a11y_media_has_caption: live streams do not expose a separate VTT captions track -->
					<video
						slot="media"
						bind:this={video}
						autoplay
						playsinline
						use:pictureInPictureEvents
						oncanplay={onCanPlay}
						onplay={() => (playbackPaused = false)}
						onplaying={onPlaying}
						onpause={() => (playbackPaused = true)}
						onwaiting={onWaiting}
						onstalled={onWaiting}
						onerror={onVideoError}
						aria-label={`${activeChannel?.name ?? 'Xivi'} live stream`}
					></video>
					<media-control-bar class="media-control-bar"
						><media-play-button></media-play-button><media-mute-button
						></media-mute-button><media-volume-range></media-volume-range><media-time-range
						></media-time-range><media-pip-button></media-pip-button><media-fullscreen-button
						></media-fullscreen-button></media-control-bar
					>
				</media-controller>
			{/if}
			{#if playerRoute && channelQuery.isPending}<div class="player-loading">
					<Radio size={40} />
					<p>Tuning the signal…</p>
				</div>
			{:else if playerRoute && channelQuery.isError}<div class="player-error">
					<h1>Channel unavailable</h1>
					<p>This channel may have been moved or removed.</p>
					<a class="app-button app-button--primary" href="/channels">Back to channels</a>
				</div>
			{:else if playerRoute && activeChannel && !playbackController.native}
				{#if loading && !error}<div class="stream-overlay">
						<span></span>
						<p>Tuning {activeChannel.name}…</p>
					</div>{/if}
				{#if playbackPaused && !loading && !error}<div class="stream-overlay play-prompt">
						<p>The live signal is ready when you are.</p>
						<button class="app-button app-button--primary" onclick={() => void startPlayback()}
							><Play size={18} fill="currentColor" />Start watching</button
						>
					</div>{/if}
				{#if error}<div class="stream-overlay error">
						<h2>Signal interrupted</h2>
						<p>{error}</p>
						{#if incidentId}<p class="incident-reference">
								Diagnostic incident <a
									href={`/studio/streams?stream_id=${streamId(activeChannel.stream_url)}`}
									>{incidentId}</a
								>
							</p>{/if}
						<button
							class="app-button app-button--primary"
							onclick={() => {
								attachedUrl = '';
								void attach(activeChannel!.stream_url);
							}}><RotateCcw size={18} />Retry</button
						>
					</div>{/if}
			{/if}
		</div>
		{#if playerRoute && activeChannel}<div class="now-playing">
				<LogoTile src={activeChannel.logo_url} name={activeChannel.name} size="lg" unbounded />
				<div class="station">
					<span class="tabular">Channel {activeChannel.number} · {activeChannel.group_name}</span>
					<h1>{activeChannel.name}</h1>
				</div>
				<div class="show">
					<p class="eyebrow">On now</p>
					<h2>{activeChannel.current?.title ?? 'Schedule unavailable'}</h2>
					{#if activeChannel.current}<p>
							{programmeTime(activeChannel.current.start)}–{programmeTime(
								activeChannel.current.end
							)}{activeChannel.current.description ? ` · ${activeChannel.current.description}` : ''}
						</p>{:else}<p>Playback is available even without programme data.</p>{/if}
				</div>
				<div class="channel-skip">
					<a
						class:disabled={!previous}
						aria-disabled={!previous}
						href={previous ? `/watch/channel/${previous.id}?lineup=${lineupId}` : undefined}
						data-sveltekit-replacestate
						aria-label="Previous channel"><ChevronLeft size={23} /></a
					><a
						class:disabled={!next}
						aria-disabled={!next}
						href={next ? `/watch/channel/${next.id}?lineup=${lineupId}` : undefined}
						data-sveltekit-replacestate
						aria-label="Next channel"><ChevronRight size={23} /></a
					>
				</div>
				<div class="upcoming-programmes">
					<p class="eyebrow">Up next</p>
					{#if upcoming.length}
						<ol>
							{#each upcoming as programme (programme.id)}
								<li>
									<time>{programmeTime(programme.start)}–{programmeTime(programme.end)}</time>
									<div>
										<h3>{programme.title}</h3>
										{#if programme.description}<p>{programme.description}</p>{/if}
									</div>
								</li>
							{/each}
						</ol>
					{:else}<p class="schedule-empty">No upcoming schedule available.</p>{/if}
				</div>
			</div>{/if}
	</div>
</section>
{#if playbackController.native && !playerRoute && nativeState.active}
	<aside class="native-now-playing" aria-label="Now playing">
		<div class="mini-video"><NativeVideoSurface mode="mini" /></div>
		<div class="mini-details">
			<span>Live</span><strong>{nativeState.name}</strong><small
				>{nativeState.programme ?? 'Schedule unavailable'}</small
			>
		</div>
		<button class="app-button app-button--primary" onclick={() => void returnToNativeChannel()}
			>Open</button
		>
		<button class="app-button app-button--quiet" onclick={() => void stopNativePlayback()}
			>Stop</button
		>
	</aside>
{/if}

<style>
	.player-page {
		min-height: calc(100dvh - 4.6rem);
		background: rgb(4 5 8 / 0.95);
		padding: clamp(1rem, 3vw, 2.5rem);
	}
	.native-player-launch {
		display: grid;
		height: 100%;
		place-items: center;
		align-content: center;
		gap: 0.6rem;
		padding: 1rem;
		text-align: center;
	}
	.native-player-launch h2 {
		font-size: 1.1rem;
	}
	.native-player-launch h2,
	.native-player-launch p {
		margin: 0;
	}
	.native-player-launch p {
		max-width: 34rem;
		color: var(--muted);
		font-size: 0.85rem;
	}
	.native-player-launch > div {
		display: flex;
		gap: 0.7rem;
	}
	.native-now-playing {
		position: fixed;
		z-index: 70;
		right: 1rem;
		bottom: calc(5rem + env(safe-area-inset-bottom));
		width: min(19rem, calc(100vw - 2rem));
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto auto;
		align-items: center;
		gap: 0.7rem;
		border: 1px solid var(--line);
		border-radius: 1rem;
		background: color-mix(in oklch, var(--surface-raised) 94%, transparent);
		box-shadow: 0 18px 50px rgb(0 0 0 / 0.4);
		padding: 0 0.6rem 0.6rem;
		overflow: hidden;
		backdrop-filter: blur(18px);
	}
	.native-now-playing .mini-details {
		display: grid;
		min-width: 0;
	}
	.mini-video {
		grid-column: 1 / -1;
		aspect-ratio: 16 / 9;
		margin: 0 -0.6rem;
	}
	.native-now-playing span,
	.native-now-playing small {
		color: var(--muted);
		font-size: 0.72rem;
	}
	.native-now-playing strong,
	.native-now-playing small {
		overflow-wrap: anywhere;
	}
	.player-page.pip-background {
		position: fixed;
		z-index: -1;
		inset: auto auto 0 0;
		width: 1px;
		height: 1px;
		min-height: 0;
		overflow: hidden;
		clip-path: inset(50%);
		opacity: 0;
		padding: 0;
		pointer-events: none;
	}
	.player-page.pip-background .player-dock,
	.player-page.pip-background .player-stage,
	.player-page.pip-background .media-controller {
		width: 1px;
		height: 1px;
		min-height: 0;
	}
	.player-dock {
		max-width: 92rem;
		margin: 0 auto;
		overflow: hidden;
		border: 1px solid var(--line);
		border-radius: 1.4rem;
		background: var(--surface);
		box-shadow: 0 30px 100px rgb(0 0 0 / 0.5);
	}
	.player-bar {
		display: flex;
		height: 3.4rem;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 0 0.8rem;
	}
	.player-bar button {
		display: flex;
		min-height: 2.75rem;
		align-items: center;
		gap: 0.45rem;
		border: 0;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.player-bar-actions {
		display: flex;
		align-items: center;
		gap: 0.8rem;
	}
	.player-stage {
		position: relative;
		z-index: 0;
		isolation: isolate;
		width: 100%;
		aspect-ratio: 16/9;
		max-height: 70dvh;
		overflow: hidden;
		background: #050609;
	}
	.media-controller {
		position: absolute;
		z-index: 1;
		inset: 0;
		display: block;
		width: 100%;
		height: 100%;
		--media-control-background: linear-gradient(transparent, rgb(0 0 0/0.8));
		--media-primary-color: #f7f7f2;
		--media-secondary-color: #ff6b5e;
	}
	.media-controller video {
		width: 100%;
		height: 100%;
		object-fit: contain;
	}
	.media-control-bar {
		position: relative;
		z-index: 2;
		width: 100%;
		min-height: 3.25rem;
	}
	.player-loading,
	.player-error,
	.stream-overlay {
		position: absolute;
		z-index: 4;
		inset: 0;
		display: grid;
		place-items: center;
		align-content: center;
		gap: 0.8rem;
		background: #080a0f;
		color: #f7f7f2;
		text-align: center;
	}
	.player-loading :global(svg) {
		color: var(--aqua);
		animation: pulse 1.2s infinite;
	}
	.player-loading p,
	.player-error p,
	.stream-overlay p {
		margin: 0;
		color: #a8b2c7;
	}
	.stream-overlay {
		background: rgb(5 6 9/0.7);
		pointer-events: none;
	}
	.stream-overlay > span {
		width: 2.7rem;
		height: 2.7rem;
		border: 3px solid rgb(255 255 255/0.2);
		border-top-color: var(--aqua);
		border-radius: 50%;
		animation: spin 0.9s linear infinite;
	}
	.stream-overlay.error {
		pointer-events: auto;
	}
	.stream-overlay.play-prompt {
		background: rgb(5 6 9/0.82);
		pointer-events: auto;
	}
	.stream-overlay.error h2 {
		margin: 0;
	}
	.incident-reference {
		font-size: 0.68rem;
	}
	.incident-reference a {
		color: var(--aqua);
		font-family: ui-monospace, monospace;
		text-decoration: underline;
		text-underline-offset: 0.2rem;
	}
	.now-playing {
		position: relative;
		z-index: 1;
		isolation: isolate;
		display: grid;
		grid-template-columns: auto minmax(10rem, 0.55fr) minmax(18rem, 1.4fr) auto;
		align-items: center;
		gap: 1.25rem;
		border-top: 1px solid var(--watch-card-border);
		background: var(--watch-card);
		box-shadow: 0 -12px 30px oklch(10% 0.025 264 / 0.14);
		padding: 1.25rem;
	}
	.station {
		min-width: 0;
	}
	.station span {
		color: var(--muted);
		font-size: 0.7rem;
	}
	.station h1 {
		margin: 0.15rem 0 0;
		font-size: 1.5rem;
		overflow-wrap: anywhere;
	}
	.show {
		min-width: 0;
		border-left: 1px solid var(--line);
		padding-left: 1.25rem;
	}
	.show h2 {
		margin: 0;
		font-size: 1.35rem;
		overflow-wrap: anywhere;
	}
	.show > p:last-child {
		margin: 0.3rem 0 0;
		color: var(--muted);
		font-size: 0.75rem;
		line-height: 1.6;
	}
	.upcoming-programmes {
		grid-column: 1 / -1;
		border-top: 1px solid var(--line);
		padding-top: 1rem;
	}
	.upcoming-programmes ol {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.upcoming-programmes li {
		display: grid;
		grid-template-columns: 7rem minmax(0, 1fr);
		gap: 1rem;
		padding: 0.8rem 0;
	}
	.upcoming-programmes li + li {
		border-top: 1px solid var(--line);
	}
	.upcoming-programmes time,
	.upcoming-programmes li p,
	.schedule-empty {
		color: var(--muted);
		font-size: 0.8rem;
		line-height: 1.6;
	}
	.upcoming-programmes h3 {
		margin: 0;
		font-size: 1rem;
		overflow-wrap: anywhere;
	}
	.upcoming-programmes li p {
		margin: 0.3rem 0 0;
	}
	.channel-skip {
		position: relative;
		z-index: 1;
		display: flex;
		gap: 0.4rem;
	}
	.channel-skip a {
		display: grid;
		width: 2.75rem;
		height: 2.75rem;
		place-items: center;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface-raised);
	}
	.channel-skip a.disabled {
		pointer-events: none;
		opacity: 0.35;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@keyframes pulse {
		50% {
			opacity: 0.35;
			transform: scale(0.92);
		}
	}
	@media (max-width: 760px) {
		.player-page {
			position: fixed;
			z-index: 70;
			inset: 0;
			padding: 0;
			background: #050609;
		}
		.player-dock {
			display: flex;
			width: 100%;
			height: 100dvh;
			flex-direction: column;
			border: 0;
			border-radius: 0;
		}
		.player-bar {
			padding-top: env(safe-area-inset-top);
		}
		.player-stage {
			aspect-ratio: auto;
			max-height: none;
			min-height: 0;
			flex: 1 1 0;
		}
		.now-playing {
			flex: 0 0 auto;
			grid-template-columns: auto 1fr auto;
			padding: 1rem;
		}
		.now-playing :global(.logo-tile) {
			display: none;
		}
		.show {
			grid-row: 2;
			grid-column: 1/4;
			border: 0;
			padding: 0;
		}
		.channel-skip {
			grid-row: 1;
			grid-column: 3;
		}
		.player-bar button span {
			display: none;
		}
	}
	@media (orientation: portrait) {
		.native .player-dock {
			display: flex;
			flex-direction: column;
			height: 100dvh;
		}
		.native .player-bar {
			flex-shrink: 0;
		}
		.native .player-stage {
			flex: 0 0 auto;
			aspect-ratio: 16 / 9;
			max-height: none;
		}
		.native .now-playing {
			flex: 1 1 auto;
			min-height: 0;
			overflow-y: auto;
			align-content: start;
			align-items: start;
			padding-bottom: calc(1rem + env(safe-area-inset-bottom));
			grid-template-columns: minmax(0, 1fr) auto;
		}
		.native .now-playing :global(.logo-tile) {
			display: none;
		}
		.native .station {
			grid-column: 1;
		}
		.native .channel-skip {
			grid-column: 2;
			grid-row: 1;
			flex-wrap: wrap;
			justify-content: end;
			max-width: 9rem;
		}
		.native .show {
			grid-row: 2;
			grid-column: 1 / -1;
			padding: 0;
			border: 0;
		}
	}
</style>
