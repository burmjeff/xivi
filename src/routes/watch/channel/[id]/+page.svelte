<script lang="ts">
	import 'media-chrome';
	import Hls from 'hls.js';
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onDestroy } from 'svelte';
	import { ChevronDown, ChevronLeft, ChevronRight, RotateCcw, Radio, Play } from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type { GuideChannel, Paginated } from '$lib/api/types';
	import { preferences } from '$lib/state/preferences.svelte';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';

	const channelId = Number(page.params.id);
	const channelQuery = createQuery(() => ({
		queryKey: ['watch', 'channel', channelId],
		queryFn: () => api<GuideChannel>(`/api/v2/watch/channels/${channelId}`),
		refetchInterval: 60_000
	}));
	const lineupQuery = createQuery(() => ({
		queryKey: ['watch', 'player-lineup', preferences.lineupId],
		enabled: !!preferences.lineupId,
		queryFn: () =>
			api<Paginated<GuideChannel>>(
				`/api/v2/watch/lineups/${preferences.lineupId}/channels?limit=500`
			)
	}));
	let video = $state<HTMLVideoElement>(),
		hls: Hls | null = null,
		loading = $state(true),
		error = $state(''),
		playbackPaused = $state(true),
		attachedUrl = '',
		networkRetries = 0,
		mediaRecoveries = 0,
		viewerId = '',
		incidentId = $state(''),
		lastTelemetryAt = 0,
		retryTimer: ReturnType<typeof setTimeout> | undefined,
		stallTimer: ReturnType<typeof setTimeout> | undefined;
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
	let channelIndex = $derived(
		(lineupQuery.data?.items ?? []).findIndex((item) => item.id === channelId)
	);
	let previous = $derived(channelIndex > 0 ? lineupQuery.data?.items[channelIndex - 1] : undefined);
	let next = $derived(
		channelIndex >= 0 && channelIndex < (lineupQuery.data?.items.length ?? 0) - 1
			? lineupQuery.data?.items[channelIndex + 1]
			: undefined
	);

	function cleanup() {
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
	}
	function onPlaying() {
		loading = false;
		error = '';
		playbackPaused = false;
		networkRetries = 0;
		mediaRecoveries = 0;
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
	async function attach(url: string) {
		if (!video || !url || attachedUrl === url) return;
		cleanup();
		attachedUrl = url;
		incidentId = '';
		loading = true;
		error = '';
		playbackPaused = true;
		if (Hls.isSupported()) {
			hls = new Hls({
				enableWorker: true,
				lowLatencyMode: false,
				initialLiveManifestSize: 1,
				startFragPrefetch: true,
				backBufferLength: 30,
				maxBufferLength: 40,
				maxMaxBufferLength: 60,
				maxBufferHole: 0.5,
				highBufferWatchdogPeriod: 4,
				nudgeOffset: 0.15,
				nudgeMaxRetry: 5,
				liveSyncDurationCount: 3,
				liveSyncOnStallIncrease: 1,
				liveMaxLatencyDurationCount: 8,
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
			hls.on(Hls.Events.MANIFEST_PARSED, () => {
				void startPlayback();
			});
			hls.on(Hls.Events.ERROR, (_event, data) => {
				if (!data.fatal) {
					if (data.details === Hls.ErrorDetails.BUFFER_STALLED_ERROR) {
						onWaiting();
						void reportPlayerEvent(
							url,
							'warning',
							'hls_buffer_stalled',
							'HLS.js reported a playback stall.',
							{ type: data.type, detail: data.details },
							true
						);
					}
					return;
				}
				if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
					scheduleNetworkRecovery(url);
					return;
				}
				if (data.type === Hls.ErrorTypes.MEDIA_ERROR && mediaRecoveries < 2) {
					mediaRecoveries += 1;
					loading = true;
					error = 'The player is repairing the live signal…';
					if (mediaRecoveries === 2) hls?.swapAudioCodec();
					hls?.recoverMediaError();
					void reportPlayerEvent(
						url,
						'warning',
						'hls_media_recovery',
						'HLS.js is attempting media recovery.',
						{ recovery: mediaRecoveries, detail: data.details }
					);
					return;
				}
				loading = false;
				error = 'This stream could not be decoded by the browser.';
				void reportPlayerEvent(url, 'error', 'hls_fatal_error', error, {
					type: data.type,
					detail: data.details,
					fatal: data.fatal
				});
			});
			hls.attachMedia(video);
			hls.loadSource(viewerUrl(url));
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
	$effect(() => {
		if (video && channelQuery.data?.stream_url) void attach(channelQuery.data.stream_url);
	});
	onDestroy(cleanup);
	function programmeTime(value?: string) {
		return value
			? new Intl.DateTimeFormat([], { hour: 'numeric', minute: '2-digit' }).format(new Date(value))
			: '';
	}
</script>

<svelte:head><title>{channelQuery.data?.name ?? 'Watch'} · Xivi</title></svelte:head>
<section class="player-page">
	<div class="player-dock">
		<div class="player-bar">
			<button onclick={() => (history.length > 1 ? history.back() : goto('/'))}
				><ChevronDown size={20} /><span>Collapse player</span></button
			><span class="live-pill">Live</span>
		</div>
		<div class="player-stage">
			{#if channelQuery.isPending}<div class="player-loading">
					<Radio size={40} />
					<p>Tuning the signal…</p>
				</div>
			{:else if channelQuery.isError}<div class="player-error">
					<h1>Channel unavailable</h1>
					<p>This channel may have been moved or removed.</p>
					<a class="app-button app-button--primary" href="/channels">Back to channels</a>
				</div>
			{:else if channelQuery.data}
				<media-controller class="media-controller">
					<!-- svelte-ignore a11y_media_has_caption: live streams do not expose a separate VTT captions track -->
					<video
						slot="media"
						bind:this={video}
						autoplay
						playsinline
						oncanplay={onCanPlay}
						onplay={() => (playbackPaused = false)}
						onplaying={onPlaying}
						onpause={() => (playbackPaused = true)}
						onwaiting={onWaiting}
						onstalled={onWaiting}
						onerror={() => {
							if (!hls) {
								loading = false;
								error = 'The native player lost the live signal. Retry to reconnect.';
								if (attachedUrl)
									void reportPlayerEvent(attachedUrl, 'error', 'native_media_error', error, {
										media_error_code: video?.error?.code
									});
							}
						}}
						aria-label={`${channelQuery.data.name} live stream`}
					></video>
					<media-control-bar
						><media-play-button></media-play-button><media-mute-button
						></media-mute-button><media-volume-range></media-volume-range><media-time-range
						></media-time-range><media-pip-button></media-pip-button><media-fullscreen-button
						></media-fullscreen-button></media-control-bar
					>
				</media-controller>
				{#if loading && !error}<div class="stream-overlay">
						<span></span>
						<p>Tuning {channelQuery.data.name}…</p>
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
									href={`/studio/streams?stream_id=${streamId(channelQuery.data.stream_url)}`}
									>{incidentId}</a
								>
							</p>{/if}
						<button
							class="app-button app-button--primary"
							onclick={() => {
								attachedUrl = '';
								void attach(channelQuery.data!.stream_url);
							}}><RotateCcw size={18} />Retry</button
						>
					</div>{/if}
			{/if}
		</div>
		{#if channelQuery.data}<div class="now-playing">
				<LogoTile
					src={channelQuery.data.logo_url}
					name={channelQuery.data.name}
					size="lg"
					unbounded
				/>
				<div class="station">
					<span class="tabular"
						>Channel {channelQuery.data.number} · {channelQuery.data.group_name}</span
					>
					<h1>{channelQuery.data.name}</h1>
				</div>
				<div class="show">
					<p class="eyebrow">On now</p>
					<h2>{channelQuery.data.current?.title ?? 'Schedule unavailable'}</h2>
					{#if channelQuery.data.current}<p>
							{programmeTime(channelQuery.data.current.start)}–{programmeTime(
								channelQuery.data.current.end
							)}{channelQuery.data.current.description
								? ` · ${channelQuery.data.current.description}`
								: ''}
						</p>{:else}<p>Playback is available even without programme data.</p>{/if}
				</div>
				<div class="channel-skip">
					<a
						class:disabled={!previous}
						aria-disabled={!previous}
						href={previous ? `/watch/channel/${previous.id}` : undefined}
						aria-label="Previous channel"><ChevronLeft size={23} /></a
					><a
						class:disabled={!next}
						aria-disabled={!next}
						href={next ? `/watch/channel/${next.id}` : undefined}
						aria-label="Next channel"><ChevronRight size={23} /></a
					>
				</div>
			</div>{/if}
	</div>
</section>

<style>
	.player-page {
		min-height: calc(100dvh - 4.6rem);
		background: rgb(4 5 8 / 0.95);
		padding: clamp(1rem, 3vw, 2.5rem);
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
	.player-stage {
		position: relative;
		width: 100%;
		aspect-ratio: 16/9;
		max-height: 70dvh;
		background: #050609;
	}
	.media-controller {
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
		overflow: hidden;
		margin: 0.15rem 0 0;
		font-size: 1.5rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.show {
		min-width: 0;
		border-left: 1px solid var(--line);
		padding-left: 1.25rem;
	}
	.show h2 {
		overflow: hidden;
		margin: 0;
		font-size: 1.35rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.show > p:last-child {
		overflow: hidden;
		margin: 0.3rem 0 0;
		color: var(--muted);
		font-size: 0.75rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.channel-skip {
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
			flex: 1;
		}
		.now-playing {
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
</style>
