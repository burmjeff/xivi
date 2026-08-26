<script lang="ts">
	import { createInfiniteQuery, createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Search, Grid2X2, List } from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { GuideChannel, LineupSummary, Paginated, WatchGroupSummary } from '$lib/api/types';
	import { preferences, selectLineup, setChannelView } from '$lib/state/preferences.svelte';
	import LineupPicker from '$lib/components/watch/LineupPicker.svelte';
	import ChannelDirectory from '$lib/components/watch/ChannelDirectory.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';

	let searchInput = $state(page.url.searchParams.get('q') ?? ''),
		search = $state((page.url.searchParams.get('q') ?? '').trim()),
		groupId = $state<number | null>(Number(page.url.searchParams.get('group')) || null);
	$effect(() => {
		const value = searchInput.trim();
		const timeout = setTimeout(() => (search = value), 180);
		return () => clearTimeout(timeout);
	});
	const lineupsQuery = createQuery(() => ({
		queryKey: ['watch', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/watch/lineups')
	}));
	let lineupId = $derived(preferences.lineupId ?? lineupsQuery.data?.items[0]?.id ?? null);
	$effect(() => {
		if (!preferences.lineupId && lineupId) selectLineup(lineupId);
	});
	const groupsQuery = createQuery(() => ({
		queryKey: ['watch', 'directory-groups', lineupId],
		enabled: !!lineupId,
		queryFn: () => api<Paginated<WatchGroupSummary>>(`/api/v2/watch/lineups/${lineupId}/groups`)
	}));
	const channelsQuery = createInfiniteQuery(() => ({
		queryKey: ['watch', 'directory', lineupId, groupId, search],
		enabled: !!lineupId,
		initialPageParam: '' as string,
		queryFn: ({ pageParam }) =>
			api<Paginated<GuideChannel>>(
				`/api/v2/watch/lineups/${lineupId}/channels${params({
					group_id: groupId,
					q: search,
					cursor: typeof pageParam === 'string' && pageParam ? pageParam : undefined,
					limit: 60
				})}`
			),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined
	}));
	let channels = $derived(channelsQuery.data?.pages.flatMap((result) => result.items) ?? []);
	let total = $derived(channelsQuery.data?.pages[0]?.total ?? 0);
	function setGroup(value: number | null) {
		groupId = value;
		const url = new URL(page.url);
		value ? url.searchParams.set('group', String(value)) : url.searchParams.delete('group');
		void goto(`${url.pathname}${url.search}`, { replaceState: true, noScroll: true });
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
				bind:value={searchInput}
				placeholder="Search channels"
				aria-label="Search channels"
			/></label
		>
		<div class="group-chips">
			<button class:active={groupId === null} onclick={() => setGroup(null)}>All</button
			>{#each groupsQuery.data?.items ?? [] as group (group.id)}<button
					class:active={groupId === group.id}
					onclick={() => setGroup(group.id)}>{group.name}</button
				>{/each}
		</div>
		<span class="count tabular">{total} channels</span>
	</div>
	{#if channelsQuery.isPending}<div class="loading-grid">
			{#each Array(12) as _}<div class="skeleton"></div>{/each}
		</div>
	{:else if channelsQuery.isError}<EmptyState
			title="The directory is off air"
			message="We could not load this lineup. Try again, or check source health in Studio."
			action="Check Studio"
			href="/studio"
		/>
	{:else if !total && (search || groupId)}<EmptyState
			title="No match"
			message="Try a broader search or another group."
			action="Show every channel"
			href="/channels"
		/>
	{:else if !total}<EmptyState
			title="No channels here yet"
			message="Build a lineup in Studio and it will appear here automatically."
		/>
	{:else}<ChannelDirectory
			{channels}
			{total}
			view={preferences.channelView}
			hasNextPage={channelsQuery.hasNextPage}
			isFetchingNextPage={channelsQuery.isFetchingNextPage}
			onLoadMore={() => void channelsQuery.fetchNextPage()}
		/>{/if}
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
	}
</style>
