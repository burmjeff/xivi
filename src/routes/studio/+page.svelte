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
		Cpu,
		Server
	} from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type { LineupSummary, OperationJob, Paginated, StudioOverview } from '$lib/api/types';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';
	const client = useQueryClient();
	type OverviewResponse = { summary: StudioOverview; jobs: OperationJob[] };
	type SystemResponse = {
		status: { uptime: string; cpu: string; memory: string; connections: number };
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
		staleTime: 30_000
	}));
	let copied = $state(''),
		publishing = $state<number | null>(null),
		publishMessage = $state('');
	async function copy(value: string) {
		await navigator.clipboard.writeText(new URL(value, location.origin).href);
		copied = value;
		setTimeout(() => (copied = ''), 1400);
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
	function jobLabel(job: OperationJob) {
		return `${job.kind} · ${job.resource}${job.resource_id ? ` ${job.resource_id}` : ''}`;
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
			><small>Low-confidence automatic matches</small><a href="/studio/lineups"
				>Review matches <ExternalLink size={14} /></a
			>
		</section>

		<section class="panel output-panel">
			<header>
				<div>
					<p class="eyebrow">Publication</p>
					<h2>Device outputs</h2>
				</div>
				{#if publishMessage}<span class="publish-message" aria-live="polite">{publishMessage}</span
					>{/if}
			</header>
			{#if lineupsQuery.data?.items.length}<div class="output-list">
					{#each lineupsQuery.data.items as lineup}<article>
							<div>
								<strong>{lineup.name}</strong><small
									>{lineup.channel_count} channels · {lineup.epg_coverage}% guide coverage</small
								>
							</div>
							<div class="output-links">
								<button onclick={() => copy(`/m3u/${lineup.name}.m3u`)}
									>{#if copied === `/m3u/${lineup.name}.m3u`}<Check size={16} />{:else}<Copy
											size={16}
										/>{/if}M3U</button
								><button onclick={() => copy(`/xmltv/${lineup.name}.xml`)}
									>{#if copied === `/xmltv/${lineup.name}.xml`}<Check size={16} />{:else}<Copy
											size={16}
										/>{/if}XMLTV</button
								><button
									class="publish"
									disabled={publishing === lineup.id}
									onclick={() => publish(lineup.id)}
									><RefreshCw
										class={publishing === lineup.id ? 'spin' : undefined}
										size={16}
									/>{publishing === lineup.id ? 'Publishing' : 'Publish'}</button
								>
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
					{#each overviewQuery.data.jobs as job}<article class:failed={job.status === 'failed'}>
							<span
								class:failed={job.status === 'failed'}
								class:complete={job.status === 'succeeded'}
								aria-hidden="true"
							></span>
							<div><strong>{jobLabel(job)}</strong><small>{job.message || job.status}</small></div>
							<b class="tabular" class:failed={job.status === 'failed'}
								>{job.status === 'failed' ? 'Failed' : `${job.progress}%`}</b
							>
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
							<dt>Streams</dt>
							<dd>{systemQuery.data.status.connections}</dd>
						</div>
					</dl>{:else}<p class="muted"><Cpu size={15} /> Loading host metrics…</p>{/if}
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
		gap: 0.4rem;
	}
	.output-links button {
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
