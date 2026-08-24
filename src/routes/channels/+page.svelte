<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { Search, Grid2X2, List, Play, ArrowRight } from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { GuideChannel, LineupSummary, Paginated } from '$lib/api/types';
	import { preferences, selectLineup, setChannelView } from '$lib/state/preferences.svelte';
	import LineupPicker from '$lib/components/watch/LineupPicker.svelte';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';

	let search = $state(page.url.searchParams.get('q') ?? ''),
		groupName = $state(page.url.searchParams.get('group') ?? '');
	const lineupsQuery = createQuery(() => ({
		queryKey: ['watch', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/watch/lineups')
	}));
	let lineupId = $derived(preferences.lineupId ?? lineupsQuery.data?.items[0]?.id ?? null);
	$effect(() => {
		if (!preferences.lineupId && lineupId) selectLineup(lineupId);
	});
	const allQuery = createQuery(() => ({
		queryKey: ['watch', 'directory', lineupId],
		enabled: !!lineupId,
		queryFn: () =>
			api<Paginated<GuideChannel>>(`/api/v2/watch/lineups/${lineupId}/channels?limit=500`)
	}));
	let groups = $derived([
		...new Set((allQuery.data?.items ?? []).map((channel) => channel.group_name))
	]);
	let channels = $derived(
		(allQuery.data?.items ?? []).filter(
			(channel) =>
				(!groupName || channel.group_name === groupName) &&
				(!search ||
					`${channel.name} ${channel.current?.title ?? ''} ${channel.next?.title ?? ''}`
						.toLowerCase()
						.includes(search.toLowerCase()))
		)
	);
	function time(value?: string) {
		return value
			? new Intl.DateTimeFormat([], { hour: 'numeric', minute: '2-digit' }).format(new Date(value))
			: '';
	}
</script>

<svelte:head><title>Channels · Xivi</title></svelte:head>
<section class="directory">
	<header class="directory-header">
		<div>
			<p class="eyebrow">Channel directory</p>
			<h1 class="page-title">Everything live.</h1>
			<p>Search the lineup, check what’s next, and start watching in one step.</p>
		</div>
		<div class="header-controls">
			{#if lineupsQuery.data}<LineupPicker items={lineupsQuery.data.items} value={lineupId} />{/if}
			<div class="view-toggle" aria-label="Channel view">
				<button
					class:active={preferences.channelView === 'grid'}
					onclick={() => setChannelView('grid')}
					aria-label="Grid view"><Grid2X2 size={18} /></button
				><button
					class:active={preferences.channelView === 'list'}
					onclick={() => setChannelView('list')}
					aria-label="List view"><List size={19} /></button
				>
			</div>
		</div>
	</header>
	<div class="directory-tools">
		<label
			><Search size={20} /><input
				bind:value={search}
				placeholder="Channel or programme"
				aria-label="Search channels"
			/></label
		>
		<div class="group-chips">
			<button class:active={!groupName} onclick={() => (groupName = '')}>All</button
			>{#each groups as group}<button
					class:active={groupName === group}
					onclick={() => (groupName = group)}>{group}</button
				>{/each}
		</div>
		<span class="count tabular">{channels.length} channels</span>
	</div>
	{#if allQuery.isPending}<div class="loading-grid">
			{#each Array(12) as _}<div class="skeleton"></div>{/each}
		</div>
	{:else if allQuery.isError}<EmptyState
			title="The directory is off air"
			message="We could not load this lineup. Try again, or check source health in Studio."
			action="Check Studio"
			href="/studio"
		/>
	{:else if !allQuery.data?.total}<EmptyState
			title="No channels here yet"
			message="Build a lineup in Studio and it will appear here automatically."
		/>
	{:else if !channels.length}<EmptyState
			title="No match"
			message="Try a broader search or another group."
			action="Show every channel"
			href="/channels"
		/>
	{:else}<div
			class:grid={preferences.channelView === 'grid'}
			class:list={preferences.channelView === 'list'}
			class="channel-directory"
		>
			{#each channels as channel}<article>
					<LogoTile
						src={channel.logo_url}
						name={channel.name}
						size={preferences.channelView === 'grid' ? 'lg' : 'md'}
						unbounded
					/>
					<div class="channel-copy">
						<span class="channel-meta tabular">{channel.number} · {channel.group_name}</span>
						<h2>{channel.name}</h2>
						<div class="now">
							<span class="live-pill">Live</span><strong
								>{channel.current?.title ?? 'Schedule unavailable'}</strong
							>{#if channel.current}<time
									>{time(channel.current.start)}–{time(channel.current.end)}</time
								>{/if}
						</div>
						{#if channel.next}<p>
								Next at {time(channel.next.start)} <b>{channel.next.title}</b>
							</p>{:else}<p>Upcoming schedule unavailable</p>{/if}
					</div>
					<a class="play" href={`/watch/channel/${channel.id}`} aria-label={`Play ${channel.name}`}
						><Play size={20} fill="currentColor" /><span>Play</span><ArrowRight size={17} /></a
					>
				</article>{/each}
		</div>{/if}
</section>

<style>
	.directory {
		padding: clamp(1.5rem, 5vw, 4.5rem);
	}
	.directory-header {
		display: flex;
		align-items: end;
		justify-content: space-between;
		gap: 2rem;
	}
	.directory-header h1 {
		font-size: clamp(2.5rem, 6vw, 5.7rem);
	}
	.directory-header p {
		max-width: 40rem;
		margin: 1rem 0 0;
		color: var(--muted);
	}
	.header-controls {
		display: flex;
		gap: 0.6rem;
	}
	.view-toggle {
		display: flex;
		border: 1px solid var(--line);
		border-radius: 0.85rem;
		background: var(--watch-card);
		padding: 0.25rem;
	}
	.view-toggle button {
		display: grid;
		width: 2.25rem;
		height: 2.25rem;
		place-items: center;
		border: 0;
		border-radius: 0.6rem;
		background: transparent;
		cursor: pointer;
	}
	.view-toggle button.active {
		background: var(--paper);
		color: var(--ink);
	}
	.directory-tools {
		position: sticky;
		z-index: 15;
		top: 4.6rem;
		display: flex;
		align-items: center;
		gap: 1rem;
		margin: 2.5rem calc(clamp(1.5rem, 5vw, 4.5rem) * -1) 1.5rem;
		border-block: 1px solid var(--line);
		background: color-mix(in oklch, var(--deep) 94%, transparent);
		padding: 0.7rem clamp(1.5rem, 5vw, 4.5rem);
		backdrop-filter: blur(16px);
	}
	.directory-tools > label {
		display: flex;
		min-width: min(24rem, 35vw);
		min-height: 2.75rem;
		align-items: center;
		gap: 0.55rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface);
		padding: 0 0.75rem;
		color: var(--muted);
	}
	.directory-tools input {
		width: 100%;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
	}
	.group-chips {
		display: flex;
		flex: 1;
		gap: 0.4rem;
		overflow-x: auto;
	}
	.group-chips button {
		min-height: 2.5rem;
		white-space: nowrap;
		border: 1px solid var(--line);
		border-radius: 999px;
		background: transparent;
		padding: 0.45rem 0.75rem;
		color: var(--muted);
		font-size: 0.75rem;
		font-weight: 750;
		cursor: pointer;
	}
	.group-chips button.active {
		border-color: transparent;
		background: var(--sun);
		color: var(--ink);
	}
	.count {
		white-space: nowrap;
		color: var(--muted);
		font-size: 0.75rem;
	}
	.channel-directory.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
		gap: 1rem;
	}
	.channel-directory article {
		border: 1px solid var(--line);
		background: var(--watch-card);
	}
	.channel-directory.grid article {
		display: grid;
		min-height: 23rem;
		grid-template-rows: auto 1fr auto;
		justify-items: start;
		border-radius: 1.25rem;
		padding: 1.1rem;
	}
	.channel-copy {
		min-width: 0;
		width: 100%;
	}
	.channel-meta {
		display: block;
		margin-top: 1rem;
		color: var(--muted);
		font-size: 0.7rem;
	}
	.channel-copy h2 {
		overflow: hidden;
		margin: 0.2rem 0 1.2rem;
		font-size: 1.4rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.now {
		display: grid;
		grid-template-columns: auto 1fr;
		align-items: center;
		gap: 0.45rem;
	}
	.now strong {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.now time {
		grid-column: 2;
		color: var(--muted);
		font-size: 0.72rem;
	}
	.channel-copy > p {
		overflow: hidden;
		margin: 1rem 0;
		color: var(--muted);
		font-size: 0.73rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.channel-copy > p b {
		color: var(--text);
	}
	.play {
		display: flex;
		width: 100%;
		min-height: 3rem;
		align-items: center;
		gap: 0.45rem;
		border-radius: 0.8rem;
		background: var(--coral);
		padding: 0.6rem 0.8rem;
		color: var(--ink);
		font-weight: 850;
	}
	.play span {
		flex: 1;
	}
	.channel-directory.list {
		display: grid;
		gap: 0.55rem;
	}
	.channel-directory.list article {
		display: flex;
		align-items: center;
		gap: 1rem;
		border-radius: 1rem;
		padding: 0.75rem;
	}
	.channel-directory.list .channel-copy {
		display: grid;
		grid-template-columns: minmax(10rem, 0.7fr) minmax(15rem, 1.3fr) minmax(12rem, 0.8fr);
		align-items: center;
		gap: 1rem;
	}
	.channel-directory.list .channel-meta {
		margin: 0;
	}
	.channel-directory.list h2 {
		margin: 0.15rem 0 0;
	}
	.channel-directory.list .now {
		grid-row: 1/3;
		grid-column: 2;
	}
	.channel-directory.list .channel-copy > p {
		grid-row: 1/3;
		grid-column: 3;
		margin: 0;
	}
	.channel-directory.list .play {
		width: auto;
	}
	.channel-directory.list .play span,
	.channel-directory.list .play :global(svg:last-child) {
		display: none;
	}
	.loading-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
		gap: 1rem;
	}
	.loading-grid > div {
		height: 23rem;
		border-radius: 1.25rem;
	}
	@media (max-width: 800px) {
		.directory-header {
			align-items: stretch;
			flex-direction: column;
		}
		.directory-tools {
			top: 4.1rem;
			align-items: stretch;
			flex-wrap: wrap;
		}
		.directory-tools > label {
			width: 100%;
			min-width: 0;
		}
		.count {
			display: none;
		}
		.channel-directory.list .channel-copy {
			display: block;
		}
		.channel-directory.list .now {
			display: grid;
		}
		.channel-directory.list .channel-copy > p {
			display: none;
		}
	}
	@media (max-width: 550px) {
		.directory {
			padding: 1.25rem;
		}
		.directory-tools {
			margin: 2rem -1.25rem 1rem;
			padding: 0.65rem 1.25rem;
		}
		.group-chips {
			order: 2;
			width: 100%;
		}
		.channel-directory.grid {
			grid-template-columns: 1fr;
		}
		.channel-directory.grid article {
			min-height: 20rem;
		}
	}
</style>
