<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import {
		Radio,
		Rows3,
		BookOpenCheck,
		CircleAlert,
		Copy,
		Check,
		ExternalLink,
		RefreshCw,
		RotateCcw,
		Cpu,
		Server
	} from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type { LineupSummary, OperationJob, Paginated, StudioOverview } from '$lib/api/types';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';
	import { copyText } from '$lib/browser/clipboard';
	const client = useQueryClient();
	type OverviewResponse = { summary: StudioOverview; jobs: OperationJob[] };
	type TunerDevice = {
		lineup_id: number;
		lineup_name: string;
		enabled: boolean;
		fill_missing_guide_slots: boolean;
		device_id: string;
		tuner_count: number;
		base_url: string;
		lineup_url: string;
	};
	type DeviceOutputsResponse = {
		virtual_tuner: {
			enabled_count: number;
			running: boolean;
			port: number;
			devices: TunerDevice[];
		};
	};
	type SystemResponse = {
		status: {
			uptime: string;
			cpu: string;
			memory: string;
			connections: number;
			stream_sessions: number;
			stream_reconnects: number;
			slow_client_drops: number;
			stream_bytes_proxied: number;
		};
	};
	const overviewQuery = createQuery(() => ({
		queryKey: ['studio', 'overview'],
		queryFn: () => api<OverviewResponse>('/api/v2/studio/overview'),
		refetchInterval: 30_000
	}));
	const lineupsQuery = createQuery(() => ({
		queryKey: ['studio', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/studio/lineups')
	}));
	const systemQuery = createQuery(() => ({
		queryKey: ['system', 'status'],
		queryFn: () => api<SystemResponse>('/api/system/status'),
		refetchInterval: 10_000
	}));
	const deviceOutputsQuery = createQuery(() => ({
		queryKey: ['studio', 'device-outputs'],
		queryFn: () => api<DeviceOutputsResponse>('/api/v2/studio/device-outputs')
	}));
	let copied = $state(''),
		publishing = $state<number | null>(null),
		deviceSaving = $state<string[]>([]),
		publishMessage = $state('');
	function saving(key: string) {
		return deviceSaving.includes(key);
	}
	function startSaving(key: string) {
		deviceSaving = [...deviceSaving, key];
	}
	function stopSaving(key: string) {
		deviceSaving = deviceSaving.filter((item) => item !== key);
	}
	async function copy(value: string) {
		publishMessage = '';
		try {
			await copyText(new URL(value, location.origin).href);
			copied = value;
			publishMessage = 'Output address copied.';
			setTimeout(() => {
				if (copied === value) copied = '';
			}, 1400);
		} catch {
			publishMessage = 'Clipboard access was blocked by the browser.';
		}
	}
	async function publish(id: number) {
		publishing = id;
		publishMessage = '';
		try {
			await api(`/api/v2/studio/lineups/${id}/publish`, { method: 'POST' });
			publishMessage = 'Publication is queued.';
			await client.invalidateQueries({ queryKey: ['studio', 'overview'] });
		} catch {
			publishMessage = 'Publishing could not be started.';
		} finally {
			publishing = null;
		}
	}
	async function setVirtualTunerEnabled(lineupId: number, lineupName: string, enabled: boolean) {
		const savingKey = `tuner:${lineupId}`;
		startSaving(savingKey);
		publishMessage = '';
		try {
			await api(`/api/v2/studio/device-outputs/virtual-tuner/${lineupId}`, {
				method: 'PATCH',
				body: JSON.stringify({ enabled })
			});
			await deviceOutputsQuery.refetch();
			publishMessage = enabled
				? `${lineupName} is now discoverable as a virtual tuner.`
				: `${lineupName} is no longer advertised as a virtual tuner.`;
		} catch {
			publishMessage = enabled
				? `${lineupName} could not be enabled. UDP port 65001 may already be in use.`
				: `${lineupName} could not be disabled.`;
			await deviceOutputsQuery.refetch();
		} finally {
			stopSaving(savingKey);
		}
	}
	async function setGuideFillEnabled(lineupId: number, lineupName: string, enabled: boolean) {
		const savingKey = `guide:${lineupId}`;
		startSaving(savingKey);
		publishMessage = '';
		try {
			await api(`/api/v2/studio/device-outputs/fill-missing-guide-slots/${lineupId}`, {
				method: 'PATCH',
				body: JSON.stringify({ enabled })
			});
			await deviceOutputsQuery.refetch();
			await client.invalidateQueries({ queryKey: ['studio', 'overview'] });
			publishMessage = enabled
				? `${lineupName} will fill uncovered XMLTV times with channel placeholders.`
				: `${lineupName} will publish only real guide programmes.`;
		} catch {
			publishMessage = `The guide output setting for ${lineupName} could not be saved.`;
			await deviceOutputsQuery.refetch();
		} finally {
			stopSaving(savingKey);
		}
	}
	async function rotateTunerCredential(lineupId: number, lineupName: string) {
		const savingKey = `tuner-credential:${lineupId}`;
		startSaving(savingKey);
		publishMessage = '';
		try {
			await api(`/api/v2/studio/device-outputs/virtual-tuner/${lineupId}/credential/rotate`, {
				method: 'POST'
			});
			publishMessage = `${lineupName}'s virtual tuner credential was rotated. Existing tuner URLs are revoked.`;
		} catch {
			publishMessage = `The virtual tuner credential for ${lineupName} could not be rotated. Confirm your password again and retry.`;
		} finally {
			stopSaving(savingKey);
		}
	}
	function jobLabel(job: OperationJob) {
		return `${job.kind} · ${job.resource}${job.resource_id ? ` ${job.resource_id}` : ''}`;
	}
	function jobProgressLabel(job: OperationJob) {
		if (job.status === 'failed') return 'Failed';
		if (job.status === 'succeeded') return '100%';
		if (job.status === 'queued') return 'Queued';
		if (job.progress <= 0) return 'Working';
		return `${job.progress}%`;
	}
</script>

<svelte:head><title>Studio · Xivi</title></svelte:head>
<StudioHeader
	title="Control room"
	description="A clear view of setup health, guide coverage, active work, and what Xivi is publishing."
/>
{#if overviewQuery.isPending}<div class="overview-loading">
		{#each Array(8) as _}<div class="skeleton"></div>{/each}
	</div>
{:else if overviewQuery.isError}<section class="overview-error">
		<CircleAlert size={28} />
		<div>
			<h2>Studio health is unavailable</h2>
			<p>The server is running, but its workspace summary could not be loaded.</p>
		</div>
		<button class="app-button app-button--secondary" onclick={() => overviewQuery.refetch()}
			><RefreshCw size={18} />Retry</button
		>
	</section>
{:else if overviewQuery.data}
	<div class="overview-grid">
		<section class="metric signal">
			<Radio /><span>Sources</span><strong class="tabular"
				>{overviewQuery.data.summary.source_count}</strong
			><small
				>{overviewQuery.data.summary.source_channel_count.toLocaleString()} source channels</small
			><a href="/studio/sources">Manage sources <ExternalLink size={14} /></a>
		</section>
		<section class="metric coral">
			<Rows3 /><span>Lineups</span><strong class="tabular"
				>{overviewQuery.data.summary.lineup_count}</strong
			><small
				>{overviewQuery.data.summary.lineup_channel_count.toLocaleString()} organized channels</small
			><a href="/studio/lineups">Open workbench <ExternalLink size={14} /></a>
		</section>
		<section class="metric sun">
			<BookOpenCheck /><span>Guide coverage</span><strong class="tabular"
				>{overviewQuery.data.summary.epg_coverage}%</strong
			><small
				>{overviewQuery.data.summary.mapped_channel_count.toLocaleString()} channels mapped</small
			><a href="/studio/guide-data">Diagnose guide <ExternalLink size={14} /></a>
		</section>
		<section class="metric review">
			<CircleAlert /><span>Needs review</span><strong class="tabular"
				>{overviewQuery.data.summary.review_count}</strong
			>
			<div class="review-breakdown">
				<a href="/studio/lineups?match=unmatched"
					><b class="tabular">{overviewQuery.data.summary.unmatched_count}</b><span>Unmatched</span
					></a
				><a href="/studio/lineups?match=low-confidence"
					><b class="tabular">{overviewQuery.data.summary.low_confidence_count}</b><span
						>Low confidence</span
					></a
				><a href="/studio/lineups?match=duplicate-tvg-id"
					><b class="tabular">{overviewQuery.data.summary.duplicate_tvg_id_count}</b><span
						>Duplicate guide IDs</span
					></a
				>
			</div>
			<a href="/studio/lineups">Review issues <ExternalLink size={14} /></a>
		</section>

		<section class="panel output-panel">
			<header>
				<div>
					<p class="eyebrow">Publication</p>
					<h2>Device outputs</h2>
				</div>
				<div class="output-heading-actions">
					{#if publishMessage}<span class="publish-message" aria-live="polite"
							>{publishMessage}</span
						>{/if}
					<div class="tuner-summary">
						<strong>Virtual Tuner</strong><small
							class:error={(deviceOutputsQuery.data?.virtual_tuner.enabled_count ?? 0) > 0 &&
								!deviceOutputsQuery.data?.virtual_tuner.running}
							>{deviceOutputsQuery.data?.virtual_tuner.running
								? `${deviceOutputsQuery.data.virtual_tuner.enabled_count} of ${deviceOutputsQuery.data.virtual_tuner.devices.length} lineups discoverable`
								: (deviceOutputsQuery.data?.virtual_tuner.enabled_count ?? 0) > 0
									? 'UDP port 65001 unavailable'
									: 'No lineup tuners enabled'}</small
						>
					</div>
				</div>
			</header>
			{#if lineupsQuery.data?.items.length}<div class="output-list">
					{#each lineupsQuery.data.items as lineup}{@const tuner =
							deviceOutputsQuery.data?.virtual_tuner.devices.find(
								(device) => device.lineup_id === lineup.id
							)}
						<article>
							<div>
								<strong>{lineup.name}</strong><small
									>{lineup.channel_count} channels · {lineup.epg_coverage}% guide coverage</small
								>{#if tuner?.enabled}<small class="tuner-id"
										>Virtual tuner {tuner.device_id} · {tuner.tuner_count} connections</small
									>{/if}
							</div>
							<div class="output-links">
								<label class="tuner-toggle">
									<span><strong>Tuner</strong><small>{tuner?.enabled ? 'On' : 'Off'}</small></span>
									<input
										type="checkbox"
										role="switch"
										aria-label={`Enable virtual tuner for ${lineup.name}`}
										checked={tuner?.enabled ?? false}
										disabled={saving(`tuner:${lineup.id}`) ||
											deviceOutputsQuery.isPending ||
											!tuner}
										onchange={(event) =>
											setVirtualTunerEnabled(
												lineup.id,
												lineup.name,
												(event.currentTarget as HTMLInputElement).checked
											)}
									/>
								</label>
								<label
									class="tuner-toggle"
									title="Fill uncovered XMLTV intervals without changing real guide coverage"
								>
									<span
										><strong>Guide gaps</strong><small
											>{tuner?.fill_missing_guide_slots ? 'Filled' : 'Blank'}</small
										></span
									>
									<input
										type="checkbox"
										role="switch"
										aria-label={`Fill missing XMLTV guide slots for ${lineup.name}`}
										checked={tuner?.fill_missing_guide_slots ?? true}
										disabled={saving(`guide:${lineup.id}`) ||
											deviceOutputsQuery.isPending ||
											!tuner}
										onchange={(event) =>
											setGuideFillEnabled(
												lineup.id,
												lineup.name,
												(event.currentTarget as HTMLInputElement).checked
											)}
									/>
								</label>
								<a class="secured-output" href="/account#media-keys">Create secured URLs</a>
								{#if tuner?.enabled}
									<button onclick={() => copy(tuner.base_url)}>
										{#if copied === tuner.base_url}<Check size={16} />{:else}<Copy
												size={16}
											/>{/if}Tuner
									</button>
									<button
										disabled={saving(`tuner-credential:${lineup.id}`)}
										title="Invalidate existing signed tuner stream URLs"
										onclick={() => rotateTunerCredential(lineup.id, lineup.name)}
										><RotateCcw size={16} />Rotate tuner key</button
									>
								{/if}
								<button
									class="publish"
									disabled={publishing === lineup.id}
									onclick={() => publish(lineup.id)}
								>
									<RefreshCw
										class={publishing === lineup.id ? 'spin' : undefined}
										size={16}
									/>{publishing === lineup.id ? 'Publishing' : 'Publish'}
								</button>
							</div>
						</article>{/each}
				</div>{:else}<div class="empty-inline">
					<p>No lineup outputs exist yet.</p>
					<a class="app-button app-button--primary" href="/studio/lineups">Create a lineup</a>
				</div>{/if}
		</section>

		<section class="panel jobs-panel">
			<header>
				<p class="eyebrow">Activity</p>
				<h2>Recent jobs</h2>
			</header>
			{#if overviewQuery.data.jobs.length}<div class="jobs" aria-live="polite">
					{#each overviewQuery.data.jobs as job}<article
							class:failed={job.status === 'failed'}
							class:running={job.status === 'running'}
							class:indeterminate={job.status === 'running' && job.progress <= 0}
						>
							<span
								class:failed={job.status === 'failed'}
								class:complete={job.status === 'succeeded'}
								class:running={job.status === 'running'}
								aria-hidden="true"
							></span>
							<div><strong>{jobLabel(job)}</strong><small>{job.message || job.status}</small></div>
							<b class="tabular" class:failed={job.status === 'failed'}>{jobProgressLabel(job)}</b>
						</article>{/each}
				</div>{:else}<div class="empty-inline">
					<p>No background work is running. Refreshes and publishing jobs will appear here.</p>
				</div>{/if}
			<details>
				<summary><Server size={16} />System details</summary>{#if systemQuery.data}<dl>
						<div>
							<dt>Uptime</dt>
							<dd>{systemQuery.data.status.uptime}</dd>
						</div>
						<div>
							<dt>CPU</dt>
							<dd>{systemQuery.data.status.cpu}</dd>
						</div>
						<div>
							<dt>Memory</dt>
							<dd>{systemQuery.data.status.memory}</dd>
						</div>
						<div>
							<dt>Stream viewers</dt>
							<dd>{systemQuery.data.status.connections}</dd>
						</div>
						<div>
							<dt>Shared producers</dt>
							<dd>{systemQuery.data.status.stream_sessions}</dd>
						</div>
						<div>
							<dt>Source failovers</dt>
							<dd>{systemQuery.data.status.stream_reconnects}</dd>
						</div>
						<div>
							<dt>Slow viewers dropped</dt>
							<dd>{systemQuery.data.status.slow_client_drops}</dd>
						</div>
					</dl>{:else}<p class="muted"><Cpu size={15} /> Loading host metrics…</p>{/if}
				<a class="diagnostics-link" href="/studio/streams"
					><Radio size={15} />Open stream diagnostics</a
				>
			</details>
		</section>
	</div>
{/if}

<style>
	.overview-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 1rem;
		padding: 0 clamp(1.4rem, 3vw, 2.5rem) 3rem;
	}
	.metric {
		position: relative;
		display: grid;
		min-height: 15rem;
		grid-template-columns: 1fr auto;
		align-content: start;
		border-radius: 1.2rem;
		padding: 1.1rem;
		color: var(--ink);
	}
	.metric.signal {
		background: var(--aqua);
	}
	.metric.coral {
		background: var(--coral);
	}
	.metric.sun {
		background: var(--sun);
	}
	.metric.review {
		background: var(--paper);
	}
	.metric > :global(svg) {
		grid-column: 2;
		grid-row: 1;
	}
	.metric > span {
		grid-column: 1;
		grid-row: 1;
		font-weight: 820;
	}
	.metric > strong {
		grid-column: 1/3;
		margin: 1.2rem 0 0.2rem;
		font-family: var(--font-display);
		font-size: 3.4rem;
		letter-spacing: -0.06em;
	}
	.metric > small {
		grid-column: 1/3;
	}
	.metric > .review-breakdown {
		display: grid;
		grid-column: 1/3;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.45rem;
		margin-top: 0.25rem;
	}
	.review-breakdown a {
		display: grid;
		min-height: 2.75rem;
		align-content: center;
		border: 1px solid color-mix(in oklch, var(--ink) 15%, transparent);
		border-radius: 0.65rem;
		background: color-mix(in oklch, var(--ink) 5%, transparent);
		padding: 0.35rem 0.5rem;
		color: var(--ink);
		text-decoration: none;
	}
	.review-breakdown a:hover {
		background: color-mix(in oklch, var(--ink) 10%, transparent);
	}
	.review-breakdown b {
		font-size: 0.85rem;
	}
	.review-breakdown span {
		font-size: 0.6rem;
		font-weight: 750;
	}
	.metric > a {
		display: inline-flex;
		min-height: 1.5rem;
		grid-column: 1/3;
		align-items: center;
		gap: 0.35rem;
		margin-top: auto;
		font-size: 0.75rem;
		font-weight: 850;
	}
	.output-panel {
		grid-column: 1/4;
		overflow: hidden;
	}
	.output-heading-actions {
		display: flex;
		align-items: center;
		gap: 0.8rem;
	}
	.tuner-toggle {
		display: flex;
		min-height: 2.7rem;
		align-items: center;
		gap: 0.7rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface-raised);
		padding: 0.4rem 0.65rem;
		cursor: pointer;
	}
	.tuner-summary {
		display: grid;
		text-align: right;
	}
	.tuner-summary strong {
		font-size: 0.68rem;
	}
	.tuner-summary small {
		color: var(--muted);
		font-size: 0.56rem;
	}
	.tuner-summary small.error {
		color: var(--error);
	}
	.tuner-toggle span {
		display: grid;
	}
	.tuner-toggle strong {
		font-size: 0.68rem;
	}
	.tuner-toggle small {
		color: var(--muted);
		font-size: 0.56rem;
	}
	.tuner-toggle input {
		position: relative;
		box-sizing: border-box;
		width: 2.6rem;
		height: 1.4rem;
		appearance: none;
		border: 1px solid color-mix(in oklch, var(--muted) 55%, var(--line));
		border-radius: 999px;
		background: var(--line);
		cursor: pointer;
		transition:
			background 120ms ease-out,
			border-color 120ms ease-out;
	}
	.tuner-toggle input::after {
		position: absolute;
		top: 0.15rem;
		left: 0.15rem;
		width: 0.98rem;
		height: 0.98rem;
		border-radius: 50%;
		background: var(--surface);
		box-shadow: 0 1px 3px color-mix(in oklch, var(--ink) 24%, transparent);
		content: '';
		transition: transform 120ms ease-out;
	}
	.tuner-toggle input:checked {
		border-color: var(--aqua);
		background: var(--aqua);
	}
	.tuner-toggle input:checked::after {
		transform: translateX(1.18rem);
	}
	.tuner-toggle input:focus-visible {
		outline: 3px solid color-mix(in oklch, var(--periwinkle) 48%, transparent);
		outline-offset: 2px;
	}
	.tuner-toggle input:disabled {
		cursor: wait;
		opacity: 0.65;
	}
	.tuner-id {
		color: var(--periwinkle) !important;
		font-family: var(--font-mono, monospace);
	}
	.jobs-panel {
		grid-column: 4;
	}
	.panel > header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 1rem 1.2rem;
	}
	.panel h2 {
		margin: 0;
		font-size: 1.35rem;
	}
	.publish-message {
		color: var(--aqua);
		font-size: 0.75rem;
	}
	.output-list article {
		display: flex;
		align-items: center;
		gap: 1rem;
		border-bottom: 1px solid var(--line);
		padding: 0.9rem 1.2rem;
	}
	.output-list article > div:first-child {
		display: grid;
		min-width: 0;
		flex: 1;
	}
	.output-list small {
		color: var(--muted);
		font-size: 0.7rem;
	}
	.output-links {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 0.4rem;
	}
	.output-links button,
	.output-links .secured-output {
		display: flex;
		min-height: 2.5rem;
		align-items: center;
		gap: 0.35rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface-raised);
		padding: 0.45rem 0.65rem;
		font-size: 0.72rem;
		font-weight: 750;
		cursor: pointer;
		text-decoration: none;
		color: inherit;
	}
	.output-links button.publish {
		background: var(--periwinkle);
		color: #fff;
	}
	.jobs article {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		border-bottom: 1px solid var(--line);
		padding: 0.8rem 1rem;
	}
	.jobs article > span {
		width: 0.55rem;
		height: 0.55rem;
		border-radius: 50%;
		background: var(--sun);
	}
	.jobs article > span.complete {
		background: var(--success);
	}
	.jobs article > span.failed {
		background: var(--error);
	}
	.jobs article > span.running {
		animation: job-pulse 1.4s ease-out infinite;
	}
	.jobs article.indeterminate > span.running {
		box-shadow: 0 0 0 0.35rem color-mix(in oklch, var(--sun) 12%, transparent);
	}
	.jobs article.failed {
		align-items: flex-start;
		background: color-mix(in oklch, var(--error) 9%, transparent);
	}
	.jobs article > div {
		display: grid;
		min-width: 0;
		flex: 1;
	}
	.jobs strong {
		font-size: 0.75rem;
	}
	.jobs small {
		overflow: hidden;
		color: var(--muted);
		font-size: 0.65rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.jobs article.failed small {
		overflow: visible;
		color: var(--error);
		overflow-wrap: anywhere;
		white-space: normal;
	}
	.jobs b {
		font-size: 0.7rem;
	}
	.jobs b.failed {
		color: var(--error);
	}
	.jobs article.running b {
		color: color-mix(in oklch, var(--sun) 75%, var(--text));
	}
	@keyframes job-pulse {
		0%,
		100% {
			transform: scale(0.82);
			opacity: 0.72;
		}
		50% {
			transform: scale(1.12);
			opacity: 1;
		}
	}
	details {
		border-top: 1px solid var(--line);
		padding: 1rem;
	}
	summary {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		cursor: pointer;
		font-size: 0.75rem;
		font-weight: 800;
	}
	dl {
		display: grid;
		gap: 0.4rem;
		margin: 0.8rem 0 0;
	}
	dl > div {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		font-size: 0.7rem;
	}
	dt {
		color: var(--muted);
	}
	dd {
		margin: 0;
		text-align: right;
	}
	.diagnostics-link {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		margin-top: 0.8rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface-raised);
		padding: 0.65rem;
		color: var(--aqua);
		font-size: 0.68rem;
		font-weight: 800;
	}
	.empty-inline {
		display: flex;
		min-height: 10rem;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		padding: 2rem;
		color: var(--muted);
	}
	.overview-error {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin: 0 2.5rem;
		border: 1px solid color-mix(in oklch, var(--error) 50%, var(--line));
		border-radius: 1rem;
		background: var(--surface);
		padding: 1rem;
	}
	.overview-error > div {
		flex: 1;
	}
	.overview-error h2,
	.overview-error p {
		margin: 0;
	}
	.overview-error p {
		color: var(--muted);
	}
	.overview-loading {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 1rem;
		padding: 0 2.5rem;
	}
	.overview-loading div {
		height: 15rem;
		border-radius: 1.2rem;
	}
	.overview-loading div:nth-child(n + 5) {
		grid-column: span 2;
	}
	@media (max-width: 1150px) {
		.overview-grid {
			grid-template-columns: repeat(2, 1fr);
		}
		.output-panel,
		.jobs-panel {
			grid-column: span 2;
		}
	}
	@media (max-width: 650px) {
		.overview-grid {
			grid-template-columns: 1fr;
			padding: 0 1rem 2rem;
		}
		.metric,
		.output-panel,
		.jobs-panel {
			grid-column: 1;
		}
		.output-list article {
			align-items: flex-start;
			flex-direction: column;
		}
		.output-panel > header,
		.output-heading-actions {
			width: 100%;
			align-items: stretch;
			flex-direction: column;
		}
		.tuner-toggle {
			justify-content: space-between;
		}
		.output-links {
			width: 100%;
			flex-wrap: wrap;
		}
		.output-links button {
			flex: 1;
		}
		.overview-loading {
			grid-template-columns: 1fr;
			padding: 0 1rem;
		}
		.overview-loading div:nth-child(n + 5) {
			grid-column: 1;
		}
	}
</style>
