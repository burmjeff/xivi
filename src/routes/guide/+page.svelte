<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Search, ChevronLeft, ChevronRight, CalendarDays } from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { GuideChannel, LineupSummary, Paginated } from '$lib/api/types';
	import { preferences, selectLineup } from '$lib/state/preferences.svelte';
	import LineupPicker from '$lib/components/watch/LineupPicker.svelte';
	import VirtualGuide from '$lib/components/watch/VirtualGuide.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';

	const initialDay = Number(page.url.searchParams.get('day') ?? 0);
	let day = $state(Number.isFinite(initialDay) ? initialDay : 0),
		search = $state(page.url.searchParams.get('q') ?? ''),
		groupId = $state<number | null>(Number(page.url.searchParams.get('group')) || null);
	const lineupsQuery = createQuery(() => ({
		queryKey: ['watch', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/watch/lineups')
	}));
	let lineupId = $derived(preferences.lineupId ?? lineupsQuery.data?.items[0]?.id ?? null);
	$effect(() => {
		if (!preferences.lineupId && lineupId) selectLineup(lineupId);
	});
	let from = $derived.by(() => {
		const value = new Date();
		value.setHours(0, 0, 0, 0);
		value.setDate(value.getDate() + day);
		return value;
	});
	let to = $derived.by(() => {
		const value = new Date(from);
		value.setDate(value.getDate() + 1);
		return value;
	});
	const groupsQuery = createQuery(() => ({
		queryKey: ['watch', 'guide-groups', lineupId],
		enabled: !!lineupId,
		queryFn: () =>
			api<Paginated<GuideChannel>>(`/api/v2/watch/lineups/${lineupId}/channels?limit=500`)
	}));
	const guideQuery = createQuery(() => ({
		queryKey: ['watch', 'guide', lineupId, day, groupId, search],
		enabled: !!lineupId,
		queryFn: () =>
			api<Paginated<GuideChannel>>(
				`/api/v2/watch/lineups/${lineupId}/guide${params({ from: from.toISOString(), to: to.toISOString(), group_id: groupId, q: search, limit: 500 })}`
			)
	}));
	let groups = $derived.by(() => [
		...new Map(
			(groupsQuery.data?.items ?? []).map((channel) => [channel.group_id, channel.group_name])
		).entries()
	]);
	function setDay(value: number) {
		day = value;
		const url = new URL(page.url);
		value ? url.searchParams.set('day', String(value)) : url.searchParams.delete('day');
		void goto(`${url.pathname}${url.search}`, { replaceState: true, noScroll: true });
	}
	function setGroup(value: number | null) {
		groupId = value;
		const url = new URL(page.url);
		value ? url.searchParams.set('group', String(value)) : url.searchParams.delete('group');
		void goto(`${url.pathname}${url.search}`, { replaceState: true, noScroll: true });
	}
</script>

<svelte:head><title>Guide · Xivi</title></svelte:head>
<section class="guide-page">
	<header class="guide-header">
		<div>
			<p class="eyebrow">Live guide</p>
			<h1>Find your moment</h1>
		</div>
		<div class="guide-actions">
			{#if lineupsQuery.data}<LineupPicker
					items={lineupsQuery.data.items}
					value={lineupId}
				/>{/if}<label class="guide-search"
				><Search size={18} /><input
					bind:value={search}
					placeholder="Search the guide"
					aria-label="Search the guide"
				/></label
			>
		</div>
	</header>
	<div class="guide-filters">
		<div class="date-nav">
			<button onclick={() => setDay(day - 1)} aria-label="Previous day"
				><ChevronLeft size={18} /></button
			><button class="date-label" onclick={() => setDay(0)}
				><CalendarDays size={17} /><span
					>{day === 0
						? 'Today'
						: new Intl.DateTimeFormat([], {
								weekday: 'short',
								month: 'short',
								day: 'numeric'
							}).format(from)}</span
				></button
			><button onclick={() => setDay(day + 1)} aria-label="Next day"
				><ChevronRight size={18} /></button
			>
		</div>
		<div class="chips">
			<button class:active={groupId === null} onclick={() => setGroup(null)}>All channels</button
			>{#each groups as [id, name]}<button
					class:active={groupId === id}
					onclick={() => setGroup(id)}>{name}</button
				>{/each}
		</div>
	</div>
	{#if !lineupsQuery.isPending && !lineupsQuery.data?.total}<EmptyState
			title="No guide—yet"
			message="Create a lineup in Studio to start building your live guide."
		/>
	{:else if guideQuery.isPending}<div class="guide-loading skeleton"></div>
	{:else if guideQuery.isError}<EmptyState
			title="The guide missed its cue"
			message="Guide data could not be loaded. Channels can still be played from the directory."
			action="Open channels"
			href="/channels"
		/>
	{:else if !guideQuery.data?.total}<EmptyState
			title="Nothing matches"
			message="Try another group, date, or search. Missing schedules never block playback."
			action="Clear filters"
			href="/guide"
		/>
	{:else}{#key `${guideQuery.data.total}-${lineupId}`}<VirtualGuide
				channels={guideQuery.data.items}
				{from}
				{to}
			/>{/key}{/if}
</section>

<style>
	.guide-page {
		min-height: 100%;
		background: var(--deep);
	}
	.guide-header {
		display: flex;
		align-items: end;
		justify-content: space-between;
		gap: 1.5rem;
		padding: 2.25rem clamp(1rem, 3vw, 2.5rem) 1.3rem;
	}
	.guide-header h1 {
		margin: 0;
		font-size: clamp(2rem, 4vw, 3.4rem);
	}
	.guide-actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.guide-search {
		display: flex;
		min-height: 2.75rem;
		align-items: center;
		gap: 0.55rem;
		border: 1px solid var(--line);
		border-radius: 0.85rem;
		background: var(--surface);
		padding: 0 0.75rem;
		color: var(--muted);
	}
	.guide-search input {
		width: min(16rem, 25vw);
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
	}
	.guide-filters {
		display: flex;
		align-items: center;
		gap: 1rem;
		border-top: 1px solid var(--line);
		border-bottom: 1px solid var(--line);
		padding: 0.65rem clamp(1rem, 3vw, 2.5rem);
		background: var(--surface);
	}
	.date-nav {
		display: flex;
		align-items: center;
	}
	.date-nav button {
		display: grid;
		min-width: 2.75rem;
		height: 2.75rem;
		place-items: center;
		border: 1px solid var(--line);
		background: var(--surface-raised);
		cursor: pointer;
	}
	.date-nav button:first-child {
		border-radius: 0.7rem 0 0 0.7rem;
	}
	.date-nav button:last-child {
		border-radius: 0 0.7rem 0.7rem 0;
	}
	.date-nav .date-label {
		display: flex;
		width: auto;
		gap: 0.45rem;
		border-inline: 0;
		padding: 0 0.7rem;
		font-weight: 750;
	}
	.chips {
		display: flex;
		gap: 0.4rem;
		overflow-x: auto;
	}
	.chips button {
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
	.chips button.active {
		background: var(--aqua);
		color: var(--ink);
		border-color: transparent;
	}
	.guide-loading {
		height: calc(100dvh - 14rem);
	}
	@media (max-width: 760px) {
		.guide-header {
			align-items: stretch;
			flex-direction: column;
			padding-top: 1.4rem;
		}
		.guide-actions {
			display: grid;
			grid-template-columns: auto 1fr;
		}
		.guide-search input {
			width: 100%;
		}
		.guide-filters {
			align-items: flex-start;
			flex-direction: column;
		}
		.chips {
			width: 100%;
		}
	}
</style>
