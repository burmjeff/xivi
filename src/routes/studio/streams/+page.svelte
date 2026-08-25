<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import {
		Activity,
		ArrowDownToLine,
		ArrowUpFromLine,
		CircleStop,
		Clock3,
		Copy,
		Gauge,
		History,
		MonitorPlay,
		RefreshCw,
		RotateCw,
		SkipForward,
		TriangleAlert,
		Unplug,
		Users
	} from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type {
		StudioStream as GeneratedStudioStream,
		StudioStreamsResponse as GeneratedStreamsResponse,
		StreamConnection,
		StreamEvent,
		StreamMetricSample
	} from '$lib/api/types';
	import { copyText } from '$lib/browser/clipboard';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';

	type Connection = StreamConnection;
	type Sample = StreamMetricSample;
	type StreamSession = GeneratedStudioStream & {
		ingress_bitrate_bps: number;
		egress_bitrate_bps: number;
		bytes_delivered: number;
		connections: Connection[];
		samples: Sample[];
		events: StreamEvent[];
	};
	type StreamsResponse = Omit<GeneratedStreamsResponse, 'items'> & { items: StreamSession[] };
	type HistoryDetail = { connections: Connection[]; events: StreamEvent[] };

	const queryClient = useQueryClient();
	const streamsQuery = createQuery(() => ({
		queryKey: ['studio', 'streams'],
		queryFn: () => api<StreamsResponse>('/api/v2/studio/streams'),
		refetchInterval: 2_000
	}));
	let selectedIncident = $state('');
	let selectedStreamID = $state(page.url.searchParams.get('stream_id') ?? '');
	let busy = $state('');
	let message = $state('');
	let messageError = $state(false);
	const requestedStream = $derived(page.url.searchParams.get('stream_id') ?? '');
	$effect(() => {
		const items = streamsQuery.data?.items ?? [];
		if (!items.length) {
			selectedStreamID = '';
		} else if (requestedStream && items.some((item) => item.id === requestedStream)) {
			selectedStreamID = requestedStream;
		} else if (!items.some((item) => item.id === selectedStreamID)) {
			selectedStreamID = items[0].id;
		}
	});
	const selected = $derived(streamsQuery.data?.items.find((item) => item.id === selectedStreamID));
	const selectedConnections = $derived.by(() =>
		[...(selected?.connections ?? [])].sort((left, right) => {
			const started = new Date(left.started_at).getTime() - new Date(right.started_at).getTime();
			return started || left.id.localeCompare(right.id);
		})
	);
	const historyDetail = createQuery(() => ({
		queryKey: ['studio', 'streams', 'history', selectedIncident],
		queryFn: () =>
			api<HistoryDetail>(`/api/v2/studio/streams/history/${encodeURIComponent(selectedIncident)}`),
		enabled: Boolean(selectedIncident)
	}));

	function formatBytes(value = 0) {
		if (value < 1024) return `${value} B`;
		const units = ['KB', 'MB', 'GB', 'TB'];
		let amount = value / 1024;
		let unit = units[0];
		for (let index = 1; index < units.length && amount >= 1024; index++) {
			amount /= 1024;
			unit = units[index];
		}
		return `${amount >= 100 ? amount.toFixed(0) : amount.toFixed(1)} ${unit}`;
	}
	function formatBitrate(value: number | undefined = 0) {
		if (value < 1_000) return `${Math.round(value)} bps`;
		if (value < 1_000_000) return `${(value / 1_000).toFixed(0)} Kbps`;
		return `${(value / 1_000_000).toFixed(2)} Mbps`;
	}
	function logoURL(value?: string) {
		if (!value || /^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(value)) return value;
		return `/${value.replace(/^\/+/, '')}`;
	}
	function duration(from: string, to = Date.now()) {
		const seconds = Math.max(0, Math.floor((to - new Date(from).getTime()) / 1000));
		if (seconds < 60) return `${seconds}s`;
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
		return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
	}
	function programmeProgress(stream: StreamSession) {
		if (!stream.programme_start || !stream.programme_end) return 0;
		const start = new Date(stream.programme_start).getTime();
		const end = new Date(stream.programme_end).getTime();
		return Math.max(0, Math.min(100, ((Date.now() - start) / Math.max(1, end - start)) * 100));
	}
	function sessionEvents(incidentID: string) {
		return streamsQuery.data?.items.find((item) => item.incident_id === incidentID)?.events ?? [];
	}
	function eventDetails(value?: string) {
		if (!value) return '';
		try {
			return JSON.stringify(JSON.parse(value), null, 2);
		} catch {
			return value;
		}
	}
	function bitrateChart(samples: Sample[]) {
		const recent = samples.slice(-120);
		const measuredPeak = Math.max(
			0,
			...recent.flatMap((sample) => [sample.ingress_bps, sample.egress_bps])
		);
		const magnitude = 10 ** Math.floor(Math.log10(Math.max(1_000, measuredPeak)));
		const normalized = Math.max(1_000, measuredPeak) / magnitude;
		const factor = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
		const peak = factor * magnitude;
		const points = (field: 'ingress_bps' | 'egress_bps') =>
			recent
				.map((sample, index) => {
					const x = recent.length === 1 ? 100 : (index / (recent.length - 1)) * 100;
					const y = 38 - (sample[field] / peak) * 36;
					return `${x.toFixed(2)},${y.toFixed(2)}`;
				})
				.join(' ');
		return {
			sampleCount: recent.length,
			ingress: points('ingress_bps'),
			egress: points('egress_bps'),
			ticks: Array.from({ length: 5 }, (_, index) => ({
				y: 2 + index * 9,
				value: peak * (1 - index / 4)
			}))
		};
	}
	const chart = $derived.by(() => bitrateChart(selected?.samples ?? []));
	function selectStream(id: string) {
		selectedStreamID = id;
		const url = new URL(page.url);
		url.searchParams.set('stream_id', id);
		void goto(`${url.pathname}${url.search}`, {
			replaceState: true,
			keepFocus: true,
			noScroll: true
		});
	}
	async function action(streamID: string, kind: 'restart' | 'failover' | 'stop') {
		busy = kind;
		message = '';
		messageError = false;
		try {
			await api(
				`/api/v2/studio/streams/${encodeURIComponent(streamID)}${kind === 'stop' ? '' : `/${kind}`}`,
				{
					method: kind === 'stop' ? 'DELETE' : 'POST'
				}
			);
			message =
				kind === 'stop'
					? 'Stop requested. Viewers were disconnected while upstream cleanup finishes.'
					: kind === 'restart'
						? 'The active source is restarting.'
						: 'Xivi is advancing to the next ordered source.';
			await queryClient.invalidateQueries({ queryKey: ['studio', 'streams'] });
		} catch (error) {
			messageError = true;
			message = error instanceof Error ? error.message : 'The stream action failed.';
		} finally {
			busy = '';
		}
	}
	async function disconnect(streamID: string, connectionID: string) {
		busy = connectionID;
		message = '';
		try {
			await api(
				`/api/v2/studio/streams/${encodeURIComponent(streamID)}/connections/${encodeURIComponent(connectionID)}`,
				{ method: 'DELETE' }
			);
			message = 'The viewer was disconnected.';
			await queryClient.invalidateQueries({ queryKey: ['studio', 'streams'] });
		} catch (error) {
			messageError = true;
			message = error instanceof Error ? error.message : 'The viewer could not be disconnected.';
		} finally {
			busy = '';
		}
	}
	async function copyDiagnostics(stream: StreamSession) {
		const events = streamsQuery.data?.recent_events.filter(
			(event) => event.incident_id === stream.incident_id
		);
		await copyText(
			JSON.stringify(
				{
					incident_id: stream.incident_id,
					stream_id: stream.id,
					state: stream.state,
					source: `${stream.source_position}/${stream.source_count}`,
					reconnects: stream.reconnects,
					last_error: stream.last_error,
					connections: stream.connections,
					recent_events: events
				},
				null,
				2
			)
		);
		messageError = false;
		message = 'Diagnostics copied without source credentials.';
	}
</script>

<svelte:head><title>Streams · Xivi</title></svelte:head>

<StudioHeader
	eyebrow="Operations"
	title="Streams"
	description="See what every shared producer and viewer is doing, intervene safely, and keep a durable record when playback fails."
>
	<button
		class="app-button app-button--secondary"
		onclick={() => streamsQuery.refetch()}
		disabled={streamsQuery.isFetching}
	>
		<RefreshCw size={17} class={streamsQuery.isFetching ? 'spin' : ''} />Refresh
	</button>
</StudioHeader>

{#if streamsQuery.isError}
	<section class="load-error panel">
		<TriangleAlert size={24} />
		<div>
			<h2>Stream diagnostics could not load</h2>
			<p>{streamsQuery.error.message}</p>
		</div>
		<button class="app-button app-button--secondary" onclick={() => streamsQuery.refetch()}
			>Try again</button
		>
	</section>
{:else if !streamsQuery.data}
	<div class="stream-page loading" aria-label="Loading stream diagnostics">
		<div class="summary-skeleton skeleton"></div>
		<div class="summary-skeleton skeleton"></div>
		<div class="body-skeleton skeleton"></div>
	</div>
{:else}
	<div class="stream-page">
		{#if !streamsQuery.data.proxy_enabled}
			<aside class="proxy-note">
				<TriangleAlert size={19} />
				<div>
					<strong>Proxying is off</strong><span
						>Clients are redirected upstream, so Xivi can record the handoff but cannot measure or
						terminate those connections.</span
					>
				</div>
			</aside>
		{/if}
		{#if message}<p class:error={messageError} class="action-message" aria-live="polite">
				{message}
			</p>{/if}

		<section class="summary-grid" aria-label="Live stream summary">
			<article>
				<Activity size={19} /><span>Shared producers</span><strong
					>{streamsQuery.data.summary.sessions}</strong
				><small>{streamsQuery.data.summary.reconnects} source failovers</small>
			</article>
			<article>
				<Users size={19} /><span>Active viewers</span><strong
					>{streamsQuery.data.summary.clients}</strong
				><small>{formatBytes(streamsQuery.data.summary.bytes_delivered)} delivered</small>
			</article>
			<article>
				<ArrowDownToLine size={19} /><span>Ingress</span><strong
					>{formatBitrate(streamsQuery.data.summary.ingress_bitrate_bps)}</strong
				><small>{formatBytes(streamsQuery.data.summary.bytes_published)} received</small>
			</article>
			<article>
				<ArrowUpFromLine size={19} /><span>Egress</span><strong
					>{formatBitrate(streamsQuery.data.summary.egress_bitrate_bps)}</strong
				><small>{streamsQuery.data.summary.slow_client_drops} slow viewers dropped</small>
			</article>
		</section>
		{#if streamsQuery.data.source_connections.length}
			<section class="source-budgets panel" aria-label="Upstream connection budgets">
				<header>
					<div>
						<p class="eyebrow">Source capacity</p>
						<h2>Upstream connection budgets</h2>
					</div>
					<small>Recovery and prewarming cannot exceed these limits.</small>
				</header>
				<div>
					{#each streamsQuery.data.source_connections as source}
						<article>
							<span
								><strong>{source.source_name || `Source ${source.source_id}`}</strong><em
									>{source.active}/{source.limit} active</em
								></span
							>
							<progress value={source.active} max={source.limit}></progress>
						</article>
					{/each}
				</div>
			</section>
		{/if}

		<div class="workspace">
			<section class="stream-list panel">
				<header>
					<div>
						<p class="eyebrow">Live now</p>
						<h2>Active streams</h2>
					</div>
					<span>{streamsQuery.data.items.length}</span>
				</header>
				{#if streamsQuery.data.items.length}
					<div class="stream-rows">
						{#each streamsQuery.data.items as stream (stream.id)}
							<button
								class:active={selected?.id === stream.id}
								onclick={() => selectStream(stream.id)}
							>
								<div class="channel-logo">
									<LogoTile
										src={logoURL(stream.logo_url)}
										name={stream.channel_name}
										size="sm"
										contrast
									/>
								</div>
								<div class="stream-copy">
									<strong>{stream.channel_name}</strong><span
										>{stream.programme_title || 'Schedule unavailable'}</span
									><small
										>{stream.clients} viewer{stream.clients === 1 ? '' : 's'} · {formatBitrate(
											stream.egress_bitrate_bps
										)}</small
									>
								</div>
								<i
									class:error={stream.state === 'failed'}
									class:warning={stream.state === 'reconnecting'}>{stream.state}</i
								>
								{#if stream.programme_end}<div class="programme-bar">
										<span style:width={`${programmeProgress(stream)}%`}></span>
									</div>{/if}
							</button>
						{/each}
					</div>
				{:else}
					<div class="empty">
						<MonitorPlay size={32} /><strong>No streams are active</strong>
						<p>
							Start playback from Watch or connect a device. The producer and its viewers will
							appear here immediately.
						</p>
					</div>
				{/if}
			</section>

			<section class="inspector panel">
				{#if selected}
					<header class="inspect-head">
						<div>
							<p class="eyebrow">Live inspection</p>
							<h2>{selected.channel_name}</h2>
							<span class="incident">Incident {selected.incident_id}</span>
						</div>
						<div class="inspect-actions">
							<button
								class="app-button app-button--secondary"
								onclick={() => copyDiagnostics(selected)}
								title="Copy safe diagnostics"><Copy size={16} />Copy</button
							>
							<button
								class="app-button app-button--secondary"
								onclick={() => action(selected.id, 'restart')}
								disabled={Boolean(busy)}><RotateCw size={16} />Restart</button
							>
							<button
								class="app-button app-button--secondary"
								onclick={() => action(selected.id, 'failover')}
								disabled={Boolean(busy) || selected.source_count < 2}
								><SkipForward size={16} />Next source</button
							>
							<button
								class="app-button danger"
								onclick={() => action(selected.id, 'stop')}
								disabled={Boolean(busy)}><CircleStop size={16} />Stop</button
							>
						</div>
					</header>

					<div class="programme-block">
						<div>
							<span>On now</span><strong
								>{selected.programme_title || 'Schedule unavailable'}</strong
							>{#if selected.next_title}<small>Next: {selected.next_title}</small>{/if}
						</div>
						{#if selected.programme_end}<time
								>{duration(new Date().toISOString(), new Date(selected.programme_end).getTime())} remaining</time
							>{/if}
						<div class="programme-bar">
							<span style:width={`${programmeProgress(selected)}%`}></span>
						</div>
					</div>

					<div class="signal-grid">
						<div>
							<Gauge size={17} /><span>State</span><strong
								class="state"
								class:error={selected.state === 'failed'}>{selected.state}</strong
							>
						</div>
						<div>
							<Clock3 size={17} /><span>Session time</span><strong
								>{duration(selected.started_at)}</strong
							>
						</div>
						<div>
							<ArrowDownToLine size={17} /><span>Ingress</span><strong
								>{formatBitrate(selected.ingress_bitrate_bps)}</strong
							>
						</div>
						<div>
							<ArrowUpFromLine size={17} /><span>Egress</span><strong
								>{formatBitrate(selected.egress_bitrate_bps)}</strong
							>
						</div>
					</div>

					<section class="chart-block">
						<header>
							<div>
								<h3>Bitrate</h3>
								<p>Last {Math.min(120, selected.samples.length)} seconds</p>
							</div>
							<div class="legend">
								<span class="in">Ingress</span><span class="out">Egress</span>
							</div>
						</header>
						{#if chart.sampleCount > 1}
							<div class="chart-visual">
								<div class="chart-axis" aria-hidden="true">
									{#each chart.ticks as tick}<span>{formatBitrate(tick.value)}</span>{/each}
								</div>
								<svg
									viewBox="0 0 100 40"
									role="img"
									aria-label="Ingress and egress bitrate over time"
									preserveAspectRatio="none"
								>
									{#each chart.ticks as tick}
										<line x1="0" y1={tick.y} x2="100" y2={tick.y} />
									{/each}
									<polyline class="ingress" points={chart.ingress} />
									<polyline class="egress" points={chart.egress} />
								</svg>
							</div>
						{:else}<div class="chart-empty">Collecting live samples…</div>{/if}
					</section>

					<section class="source-block">
						<header>
							<div>
								<h3>Active source</h3>
								<p>Credentials and upstream URLs are intentionally hidden.</p>
							</div>
							<b>{selected.source_position} / {selected.source_count}</b>
						</header>
						<dl>
							<div>
								<dt>Channel</dt>
								<dd>{selected.source_name || 'Unknown source channel'}</dd>
							</div>
							<div>
								<dt>Group</dt>
								<dd>{selected.source_group || '—'}</dd>
							</div>
							<div>
								<dt>Playlist</dt>
								<dd>{selected.source_playlist || '—'}</dd>
							</div>
							<div>
								<dt>Last media</dt>
								<dd>
									{selected.last_media_at
										? new Date(selected.last_media_at).toLocaleTimeString()
										: 'Waiting'}
								</dd>
							</div>
							<div>
								<dt>Reconnects</dt>
								<dd>{selected.reconnects}</dd>
							</div>
							<div>
								<dt>Received / delivered</dt>
								<dd>
									{formatBytes(selected.bytes_published)} / {formatBytes(selected.bytes_delivered)}
								</dd>
							</div>
						</dl>
						{#if selected.media_tracks?.length}<div class="media-tracks">
								{#each selected.media_tracks as track}<code>{track}</code>{/each}
							</div>{/if}
					</section>

					<section class="connections-block">
						<header>
							<div>
								<h3>Viewer connections</h3>
								<p>HLS viewers are grouped by their player ID and expire after inactivity.</p>
							</div>
							<span>{selectedConnections.length}</span>
						</header>
						{#if selectedConnections.length}
							<div class="connection-table">
								{#each selectedConnections as connection (connection.id)}
									<article>
										<div class="connection-primary">
											<b>{connection.remote_ip || 'Unknown address'}</b><span
												>{connection.protocol.toUpperCase()}</span
											>
										</div>
										<div>
											<small>Active</small><strong>{duration(connection.started_at)}</strong>
										</div>
										<div>
											<small>Rate</small><strong>{formatBitrate(connection.bitrate_bps)}</strong>
										</div>
										<div>
											<small>Delivered</small><strong
												>{formatBytes(connection.bytes_delivered)}</strong
											>
										</div>
										<p title={connection.user_agent}>{connection.user_agent || 'Unknown client'}</p>
										<button
											onclick={() => disconnect(selected.id, connection.id)}
											disabled={busy === connection.id}
											title="Disconnect viewer"><Unplug size={15} /><span>Disconnect</span></button
										>
									</article>
								{/each}
							</div>
						{:else}<p class="section-empty">
								The producer is warm, but no viewer is currently attached.
							</p>{/if}
					</section>

					<section class="events-block">
						<header>
							<div>
								<h3>Session events</h3>
								<p>Source failures remain here after a successful failover.</p>
							</div>
						</header>
						<!-- svelte-ignore a11y_no_noninteractive_tabindex (scrollable regions need keyboard focus) -->
						{#if sessionEvents(selected.incident_id).length}<div
								class="event-list"
								role="region"
								aria-label="Session events, newest first"
								tabindex="0"
							>
								{#each sessionEvents(selected.incident_id) as event (event.id)}<article
										class:error={event.severity === 'error'}
										class:warning={event.severity === 'warning'}
									>
										<i></i><time>{new Date(event.created_at).toLocaleTimeString()}</time>
										<div>
											<strong>{event.code.replaceAll('_', ' ')}</strong>
											<p>{event.message}</p>
											{#if event.details}<details class="event-details">
													<summary>Technical details</summary>
													<pre>{eventDetails(event.details)}</pre>
												</details>{/if}
										</div>
										{#if event.source_position}<span>Source {event.source_position}</span>{/if}
									</article>{/each}
							</div>{:else}<p class="section-empty">
								No warnings or errors have been recorded for this session.
							</p>{/if}
					</section>
				{:else}
					<div class="empty">
						<Activity size={32} /><strong>Select an active stream</strong>
						<p>Signal, viewer, and event details will appear here.</p>
					</div>
				{/if}
			</section>
		</div>

		<section class="history-panel panel">
			<header>
				<div>
					<p class="eyebrow">Durable diagnostics</p>
					<h2>Recent sessions</h2>
					<p>
						Fourteen days of completed and failed sessions, without high-frequency metric samples.
					</p>
				</div>
				<History size={22} />
			</header>
			{#if streamsQuery.data.history.length}<div class="history-list">
					{#each streamsQuery.data.history as item}<button
							class:active={selectedIncident === item.incident_id}
							onclick={() =>
								(selectedIncident = selectedIncident === item.incident_id ? '' : item.incident_id)}
							><i class:error={item.state === 'failed' || Boolean(item.last_error)}></i>
							<div>
								<strong>{item.channel_name || item.stream_id}</strong><span
									>{item.error_code
										? item.error_code.replaceAll('_', ' ')
										: item.end_reason || item.state}</span
								>
							</div>
							<time>{new Date(item.started_at).toLocaleString()}</time><b
								>{formatBytes(item.bytes_delivered)}</b
							></button
						>{#if selectedIncident === item.incident_id}<div class="history-detail">
								{#if historyDetail.isPending}<p>
										Loading session audit…
									</p>{:else if historyDetail.data}<div class="history-column">
										<h3>
											Connections <span>{historyDetail.data.connections.length}</span>
										</h3>
										<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
										<div
											class="history-scroll"
											role="region"
											aria-label="Completed viewer connections"
											tabindex="0"
										>
											{#if historyDetail.data.connections.length}{#each historyDetail.data.connections as connection}<p
													>
														<strong
															>{connection.remote_ip || 'Unknown IP'} · {connection.protocol.toUpperCase()}</strong
														><span
															>{formatBytes(connection.bytes_delivered)} · {connection.end_reason ||
																'ended'}</span
														>
													</p>{/each}{:else}<p>No completed viewer connections.</p>{/if}
										</div>
									</div>
									<div class="history-column">
										<h3>Events <span>{historyDetail.data.events.length}</span></h3>
										<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
										<div
											class="history-scroll"
											role="region"
											aria-label="Session events"
											tabindex="0"
										>
											{#if historyDetail.data.events.length}{#each historyDetail.data.events as event}<p
														class:error={event.severity === 'error'}
													>
														<strong>{event.code.replaceAll('_', ' ')}</strong><span
															>{event.message}</span
														>
													</p>{/each}{:else}<p>No recorded events.</p>{/if}
										</div>
									</div>{/if}
							</div>{/if}{/each}
				</div>{:else}<p class="section-empty">Completed stream sessions will appear here.</p>{/if}
		</section>
	</div>
{/if}

<style>
	.stream-page {
		display: grid;
		gap: 1rem;
		padding: 0 clamp(1rem, 3vw, 2.5rem) 3rem;
	}
	.proxy-note,
	.action-message {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		border: 1px solid color-mix(in oklch, var(--sun) 42%, var(--line));
		border-radius: 1rem;
		background: color-mix(in oklch, var(--sun) 10%, var(--surface));
		padding: 0.8rem 1rem;
	}
	.proxy-note > div {
		display: grid;
	}
	.proxy-note span {
		color: var(--muted);
		font-size: 0.72rem;
	}
	.action-message {
		margin: 0;
		border-color: color-mix(in oklch, var(--aqua) 45%, var(--line));
		background: color-mix(in oklch, var(--aqua) 9%, var(--surface));
		font-size: 0.75rem;
		font-weight: 750;
	}
	.action-message.error {
		border-color: color-mix(in oklch, var(--error) 50%, var(--line));
		background: color-mix(in oklch, var(--error) 9%, var(--surface));
		color: var(--error);
	}
	.summary-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.75rem;
	}
	.summary-grid article {
		display: grid;
		grid-template-columns: 1fr auto;
		min-height: 8.5rem;
		border: 1px solid var(--line);
		border-radius: 1.15rem;
		background: var(--surface);
		padding: 1rem;
	}
	.summary-grid article > :global(svg) {
		grid-column: 2;
		grid-row: 1;
		color: var(--aqua);
	}
	.summary-grid span {
		grid-column: 1;
		grid-row: 1;
		color: var(--muted);
		font-size: 0.7rem;
		font-weight: 750;
	}
	.summary-grid strong {
		grid-column: 1/3;
		align-self: end;
		font-family: var(--font-display);
		font-size: clamp(1.65rem, 3vw, 2.35rem);
		letter-spacing: -0.05em;
	}
	.summary-grid small {
		grid-column: 1/3;
		color: var(--muted);
		font-size: 0.65rem;
	}
	.source-budgets {
		display: grid;
		grid-template-columns: minmax(14rem, 0.6fr) 1fr;
		gap: 1rem;
		padding: 1rem;
	}
	.source-budgets > header h2,
	.source-budgets > header p {
		margin: 0;
	}
	.source-budgets > header > small {
		display: block;
		margin-top: 0.35rem;
		color: var(--muted);
		font-size: 0.66rem;
	}
	.source-budgets > div {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
		gap: 0.6rem;
	}
	.source-budgets article,
	.source-budgets article span {
		display: grid;
		gap: 0.35rem;
	}
	.source-budgets article {
		border-radius: 0.8rem;
		background: var(--surface-raised);
		padding: 0.7rem;
	}
	.source-budgets article span {
		grid-template-columns: 1fr auto;
		font-size: 0.68rem;
	}
	.source-budgets article em {
		color: var(--muted);
		font-style: normal;
	}
	.source-budgets progress {
		width: 100%;
		height: 0.35rem;
		accent-color: var(--aqua);
	}
	.workspace {
		display: grid;
		grid-template-columns: minmax(17rem, 24rem) minmax(0, 1fr);
		gap: 1rem;
		align-items: start;
	}
	.stream-list {
		position: sticky;
		top: 1rem;
		max-height: calc(100dvh - 2rem);
		overflow: auto;
	}
	.stream-list > header,
	.history-panel > header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 1rem 1.1rem;
	}
	.stream-list h2,
	.history-panel h2,
	.inspect-head h2 {
		margin: 0;
		font-size: 1.35rem;
	}
	.stream-list > header > span,
	.connections-block > header > span {
		display: grid;
		min-width: 2rem;
		height: 2rem;
		place-items: center;
		border-radius: 999px;
		background: var(--surface-raised);
		font-size: 0.7rem;
		font-weight: 850;
	}
	.stream-rows {
		display: grid;
		padding: 0.45rem;
	}
	.stream-rows > button {
		position: relative;
		display: grid;
		grid-template-columns: 3rem 1fr auto;
		gap: 0.2rem 0.7rem;
		min-width: 0;
		border: 1px solid transparent;
		border-radius: 0.9rem;
		background: transparent;
		padding: 0.7rem;
		text-align: left;
		cursor: pointer;
	}
	.stream-rows > button:hover {
		background: var(--surface-raised);
	}
	.stream-rows > button.active {
		border-color: color-mix(in oklch, var(--periwinkle) 55%, var(--line));
		background: color-mix(in oklch, var(--periwinkle) 12%, var(--surface));
	}
	.channel-logo {
		display: grid;
		width: 3rem;
		height: 3rem;
		grid-row: 1/3;
		place-items: center;
	}
	.channel-logo :global(.logo-tile) {
		box-shadow: 0 0.3rem 0.8rem color-mix(in oklch, var(--ink) 18%, transparent);
	}
	.stream-copy {
		display: grid;
		min-width: 0;
	}
	.stream-copy strong,
	.stream-copy span,
	.stream-copy small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.stream-copy strong {
		font-size: 0.78rem;
	}
	.stream-copy span,
	.stream-copy small {
		color: var(--muted);
		font-size: 0.62rem;
	}
	.stream-rows i {
		align-self: start;
		border-radius: 999px;
		background: color-mix(in oklch, var(--success) 16%, transparent);
		color: var(--success);
		padding: 0.22rem 0.4rem;
		font-size: 0.55rem;
		font-style: normal;
		font-weight: 850;
		text-transform: uppercase;
	}
	.stream-rows i.warning {
		background: color-mix(in oklch, var(--sun) 15%, transparent);
		color: var(--sun);
	}
	.stream-rows i.error {
		background: color-mix(in oklch, var(--error) 15%, transparent);
		color: var(--error);
	}
	.programme-bar {
		overflow: hidden;
		height: 0.25rem;
		border-radius: 99px;
		background: var(--line);
	}
	.stream-rows .programme-bar {
		grid-column: 2/4;
	}
	.programme-bar span {
		display: block;
		height: 100%;
		border-radius: inherit;
		background: var(--aqua);
	}
	.inspector {
		min-width: 0;
		overflow: hidden;
	}
	.inspect-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		border-bottom: 1px solid var(--line);
		padding: 1rem 1.2rem;
	}
	.incident {
		color: var(--muted);
		font-family: ui-monospace, monospace;
		font-size: 0.6rem;
	}
	.inspect-actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 0.35rem;
	}
	.inspect-actions .app-button {
		min-height: 2.4rem;
		padding: 0.45rem 0.65rem;
		font-size: 0.67rem;
	}
	.inspect-actions .danger {
		border-color: color-mix(in oklch, var(--error) 40%, var(--line));
		background: color-mix(in oklch, var(--error) 12%, var(--surface));
		color: var(--error);
	}
	.programme-block {
		display: grid;
		grid-template-columns: 1fr auto;
		gap: 0.35rem 1rem;
		border-bottom: 1px solid var(--line);
		padding: 1rem 1.2rem;
	}
	.programme-block > div:first-child {
		display: grid;
	}
	.programme-block span,
	.programme-block small,
	.programme-block time {
		color: var(--muted);
		font-size: 0.66rem;
	}
	.programme-block strong {
		font-size: 0.9rem;
	}
	.programme-block .programme-bar {
		grid-column: 1/3;
	}
	.signal-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.55rem;
		padding: 1rem 1.2rem;
	}
	.signal-grid > div {
		display: grid;
		grid-template-columns: 1fr auto;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface-raised);
		padding: 0.7rem;
	}
	.signal-grid :global(svg) {
		grid-column: 2;
		grid-row: 1/3;
		color: var(--muted);
	}
	.signal-grid span {
		color: var(--muted);
		font-size: 0.58rem;
	}
	.signal-grid strong {
		font-size: 0.74rem;
	}
	.signal-grid strong.state {
		text-transform: capitalize;
	}
	strong.error {
		color: var(--error);
	}
	.chart-block,
	.source-block,
	.connections-block,
	.events-block {
		border-top: 1px solid var(--line);
		padding: 1rem 1.2rem;
	}
	.chart-block > header,
	.source-block > header,
	.connections-block > header,
	.events-block > header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}
	.chart-block h3,
	.source-block h3,
	.connections-block h3,
	.events-block h3 {
		margin: 0;
		font-size: 1rem;
	}
	.chart-block p,
	.source-block p,
	.connections-block p,
	.events-block header p {
		margin: 0.15rem 0 0;
		color: var(--muted);
		font-size: 0.62rem;
	}
	.legend {
		display: flex;
		gap: 0.8rem;
		font-size: 0.6rem;
	}
	.legend span::before {
		display: inline-block;
		width: 0.55rem;
		height: 0.18rem;
		margin-right: 0.3rem;
		border-radius: 2px;
		background: var(--aqua);
		content: '';
		vertical-align: middle;
	}
	.legend .out::before {
		background: var(--periwinkle);
	}
	.chart-visual {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 0.65rem;
		align-items: stretch;
		margin-top: 0.7rem;
	}
	.chart-axis {
		display: flex;
		height: 11rem;
		flex-direction: column;
		justify-content: space-between;
		box-sizing: border-box;
		padding-block: 0.35rem;
		color: var(--muted);
		font-size: 0.53rem;
		font-variant-numeric: tabular-nums;
		line-height: 1;
		text-align: right;
	}
	.chart-block svg {
		width: 100%;
		height: 11rem;
		overflow: visible;
	}
	.chart-block line {
		stroke: var(--line);
		stroke-width: 0.35;
	}
	.chart-block polyline {
		fill: none;
		stroke-width: 1.25;
		vector-effect: non-scaling-stroke;
	}
	.chart-block .ingress {
		stroke: var(--aqua);
	}
	.chart-block .egress {
		stroke: var(--periwinkle);
	}
	.chart-empty {
		display: grid;
		height: 8rem;
		place-items: center;
		color: var(--muted);
		font-size: 0.7rem;
	}
	.source-block > header b {
		border-radius: 999px;
		background: var(--surface-raised);
		padding: 0.35rem 0.55rem;
		font-size: 0.66rem;
	}
	.source-block dl {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.55rem;
		margin: 0.8rem 0 0;
	}
	.source-block dl div {
		min-width: 0;
		border-radius: 0.65rem;
		background: var(--surface-raised);
		padding: 0.55rem 0.65rem;
	}
	.source-block dt {
		color: var(--muted);
		font-size: 0.56rem;
	}
	.source-block dd {
		margin: 0.15rem 0 0;
		overflow: hidden;
		font-size: 0.68rem;
		font-weight: 750;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.media-tracks {
		display: grid;
		gap: 0.3rem;
		margin-top: 0.6rem;
	}
	.media-tracks code {
		overflow: hidden;
		border-radius: 0.5rem;
		background: var(--surface-raised);
		padding: 0.45rem 0.55rem;
		color: var(--muted);
		font-size: 0.55rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.connection-table,
	.event-list {
		display: grid;
		gap: 0.4rem;
		margin-top: 0.75rem;
	}
	.events-block .event-list {
		max-height: min(28rem, 55dvh);
		overflow-y: auto;
		overscroll-behavior: contain;
		scrollbar-gutter: stable;
		padding-right: 0.35rem;
	}
	.events-block .event-list:focus-visible {
		border-radius: 0.45rem;
		outline: 2px solid var(--periwinkle);
		outline-offset: 0.2rem;
	}
	.connection-table article {
		display: grid;
		grid-template-columns: minmax(8rem, 1.3fr) repeat(3, minmax(4rem, 0.65fr)) auto;
		align-items: center;
		gap: 0.6rem;
		border: 1px solid var(--line);
		border-radius: 0.75rem;
		background: var(--surface-raised);
		padding: 0.65rem;
	}
	.connection-table article > div {
		display: grid;
	}
	.connection-primary {
		display: flex !important;
		align-items: center;
		gap: 0.4rem;
	}
	.connection-primary b {
		font-size: 0.68rem;
	}
	.connection-primary span {
		border-radius: 99px;
		background: color-mix(in oklch, var(--aqua) 14%, transparent);
		color: var(--aqua);
		padding: 0.17rem 0.3rem;
		font-size: 0.5rem;
		font-weight: 850;
	}
	.connection-table small {
		color: var(--muted);
		font-size: 0.52rem;
	}
	.connection-table strong {
		font-size: 0.62rem;
	}
	.connection-table p {
		grid-column: 1/5;
		overflow: hidden;
		margin: 0;
		color: var(--muted);
		font-size: 0.55rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.connection-table button {
		display: flex;
		grid-column: 5;
		grid-row: 1/3;
		align-items: center;
		gap: 0.3rem;
		border: 1px solid var(--line);
		border-radius: 0.55rem;
		background: var(--surface);
		padding: 0.42rem;
		font-size: 0.58rem;
		cursor: pointer;
	}
	.event-list article {
		display: grid;
		grid-template-columns: 0.45rem 5rem 1fr auto;
		align-items: start;
		gap: 0.55rem;
		border-radius: 0.65rem;
		background: var(--surface-raised);
		padding: 0.6rem;
	}
	.event-list i {
		width: 0.45rem;
		height: 0.45rem;
		margin-top: 0.15rem;
		border-radius: 50%;
		background: var(--aqua);
	}
	.event-list article.warning i {
		background: var(--sun);
	}
	.event-list article.error i {
		background: var(--error);
	}
	.event-list time,
	.event-list span {
		color: var(--muted);
		font-size: 0.56rem;
	}
	.event-list div {
		min-width: 0;
	}
	.event-list strong {
		font-size: 0.62rem;
		text-transform: capitalize;
	}
	.event-list p {
		margin: 0.1rem 0 0;
		overflow-wrap: anywhere;
		font-size: 0.6rem;
	}
	.event-details {
		margin-top: 0.35rem;
		border: 0;
		padding: 0;
	}
	.event-details summary {
		color: var(--muted);
		font-size: 0.55rem;
		cursor: pointer;
	}
	.event-details pre {
		max-height: 12rem;
		overflow: auto;
		margin: 0.35rem 0 0;
		border-radius: 0.45rem;
		background: var(--surface);
		padding: 0.5rem;
		color: var(--muted);
		font-size: 0.52rem;
		white-space: pre-wrap;
	}
	.section-empty,
	.empty {
		color: var(--muted);
		font-size: 0.7rem;
	}
	.section-empty {
		margin: 0.8rem 0 0;
	}
	.empty {
		display: grid;
		min-height: 15rem;
		place-items: center;
		align-content: center;
		gap: 0.4rem;
		padding: 2rem;
		text-align: center;
	}
	.empty p {
		max-width: 25rem;
		margin: 0;
	}
	.history-panel > header p {
		margin: 0.15rem 0 0;
		color: var(--muted);
		font-size: 0.65rem;
	}
	.history-list {
		display: grid;
		padding: 0.45rem;
	}
	.history-list > button {
		display: grid;
		grid-template-columns: 0.5rem minmax(8rem, 1fr) minmax(9rem, auto) 6rem;
		align-items: center;
		gap: 0.7rem;
		border: 0;
		border-radius: 0.65rem;
		background: transparent;
		padding: 0.65rem;
		text-align: left;
		cursor: pointer;
	}
	.history-list > button:hover,
	.history-list > button.active {
		background: var(--surface-raised);
	}
	.history-list > button > i {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 50%;
		background: var(--success);
	}
	.history-list > button > i.error {
		background: var(--error);
	}
	.history-list > button > div {
		display: grid;
	}
	.history-list strong {
		font-size: 0.7rem;
	}
	.history-list span,
	.history-list time {
		color: var(--muted);
		font-size: 0.6rem;
	}
	.history-list b {
		font-size: 0.65rem;
		text-align: right;
	}
	.history-detail {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
		margin: 0.2rem 0.65rem 0.7rem 1.8rem;
		border-left: 2px solid var(--periwinkle);
		background: var(--surface-raised);
		padding: 0.8rem;
	}
	.history-detail h3 {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin: 0;
		font-size: 0.75rem;
	}
	.history-detail h3 span {
		display: inline-grid;
		min-width: 1.25rem;
		min-height: 1.25rem;
		place-items: center;
		border-radius: 999px;
		background: color-mix(in oklch, var(--periwinkle) 12%, var(--surface));
		padding-inline: 0.3rem;
		font-size: 0.58rem;
		font-variant-numeric: tabular-nums;
	}
	.history-column {
		display: grid;
		min-width: 0;
		align-content: start;
		gap: 0.4rem;
	}
	.history-scroll {
		max-height: min(20rem, 50dvh);
		overflow-y: auto;
		overscroll-behavior: contain;
		scrollbar-gutter: stable;
		padding-right: 0.35rem;
	}
	.history-scroll:focus-visible {
		border-radius: 0.35rem;
		outline: 2px solid var(--periwinkle);
		outline-offset: 2px;
	}
	.history-detail p {
		display: grid;
		margin: 0.35rem 0;
		font-size: 0.6rem;
	}
	.history-detail p > span {
		white-space: normal;
	}
	.history-detail p.error strong {
		color: var(--error);
	}
	.load-error {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin: 0 2.5rem;
		padding: 1rem;
	}
	.load-error div {
		flex: 1;
	}
	.load-error h2,
	.load-error p {
		margin: 0;
	}
	.load-error p {
		color: var(--muted);
	}
	.loading {
		grid-template-columns: 1fr 1fr;
	}
	.summary-skeleton {
		height: 8rem;
		border-radius: 1rem;
	}
	.body-skeleton {
		grid-column: 1/3;
		height: 35rem;
		border-radius: 1.2rem;
	}
	@media (max-width: 1180px) {
		.summary-grid {
			grid-template-columns: repeat(2, 1fr);
		}
		.workspace {
			grid-template-columns: 19rem minmax(0, 1fr);
		}
		.signal-grid {
			grid-template-columns: repeat(2, 1fr);
		}
		.source-block dl {
			grid-template-columns: repeat(2, 1fr);
		}
		.inspect-head {
			align-items: stretch;
			flex-direction: column;
		}
		.inspect-actions {
			justify-content: flex-start;
		}
		.connection-table article {
			grid-template-columns: 1fr 1fr;
		}
		.connection-table p {
			grid-column: 1/3;
		}
		.connection-table button {
			grid-column: 2;
			grid-row: 1;
			justify-self: end;
		}
	}
	@media (max-width: 850px) {
		.source-budgets {
			grid-template-columns: 1fr;
		}
		.workspace {
			grid-template-columns: 1fr;
		}
		.stream-list {
			position: static;
			max-height: none;
		}
		.history-detail {
			grid-template-columns: 1fr;
		}
	}
	@media (max-width: 600px) {
		.stream-page {
			padding-inline: 0.75rem;
		}
		.summary-grid {
			grid-template-columns: 1fr 1fr;
		}
		.summary-grid article {
			min-height: 7.5rem;
		}
		.summary-grid strong {
			font-size: 1.35rem;
		}
		.signal-grid,
		.source-block dl {
			grid-template-columns: 1fr 1fr;
		}
		.inspect-actions .app-button {
			flex: 1;
		}
		.programme-block {
			grid-template-columns: 1fr;
		}
		.programme-block .programme-bar {
			grid-column: 1;
		}
		.programme-block time {
			grid-row: 2;
		}
		.connection-table article {
			grid-template-columns: 1fr 1fr;
		}
		.history-list > button {
			grid-template-columns: 0.5rem 1fr auto;
		}
		.history-list > button time {
			display: none;
		}
		.history-detail {
			margin-left: 0.8rem;
		}
	}
</style>
