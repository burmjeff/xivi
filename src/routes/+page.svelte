<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { Search, ArrowRight, Sparkles } from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type { LineupSummary, Paginated, WatchGroupSummary } from '$lib/api/types';
	import { preferences, selectLineup } from '$lib/state/preferences.svelte';
	import LineupPicker from '$lib/components/watch/LineupPicker.svelte';
	import HomeChannelRail from '$lib/components/watch/HomeChannelRail.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';

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
	const groupsQuery = createQuery(() => ({
		queryKey: ['watch', 'home-groups', lineupId],
		enabled: !!lineupId,
		queryFn: () => api<Paginated<WatchGroupSummary>>(`/api/v2/watch/lineups/${lineupId}/groups`)
	}));
</script>

<svelte:head
	><title>Watch · Xivi</title><meta
		name="description"
		content="Browse and watch your live Xivi lineup."
	/></svelte:head
>
<div class="home-page">
	<section class="hero">
		<div>
			<p class="eyebrow"><Sparkles size={13} /> Your channels, organized</p>
			<h1 class="page-title">What’s good<br />right now?</h1>
			<p class="hero-copy">
				Jump into something live, or open the guide when you want the full picture.
			</p>
		</div>
		<div class="hero-tools">
			{#if lineupsQuery.data}<LineupPicker
					items={lineupsQuery.data.items}
					value={lineupId}
					fullWidth
				/>{/if}<a class="search-cta" href="/channels"
				><Search size={19} /><span>Search channels and shows</span><ArrowRight size={18} /></a
			>
		</div>
	</section>
	{#if lineupsQuery.isPending}<div class="loading-rail">
			{#each Array(4) as _}<div class="skeleton"></div>{/each}
		</div>
	{:else if !lineupsQuery.data?.total}<EmptyState
			title="Let’s get you on the air"
			message="Add a playlist, create a lineup, and Xivi will turn it into a friendly live TV experience."
		/>
	{:else if groupsQuery.isError}<EmptyState
			title="We lost the signal"
			message="The lineup could not be loaded. Your streams and published outputs are unaffected."
			action="Try again"
			href="/"
		/>
	{:else if groupsQuery.isPending}<div class="loading-rail">
			{#each Array(4) as _}<div class="skeleton"></div>{/each}
		</div>
	{:else if !groupsQuery.data?.total}<EmptyState
			title="This lineup is quiet"
			message="Add playable channels in Studio, then come back here to watch."
			action="Edit lineup"
			href="/studio/lineups"
		/>
	{:else}<div class="rails">
			{#each groupsQuery.data?.items ?? [] as group, index (group.id)}
				<HomeChannelRail {group} lineupId={lineupId!} eager={index === 0} />
			{/each}
		</div>{/if}
</div>

<style>
	.home-page {
		width: 100%;
		min-width: 0;
		overflow-x: clip;
		padding-bottom: 4rem;
	}
	.hero {
		display: grid;
		min-height: 22rem;
		grid-template-columns: 1.25fr 0.75fr;
		align-items: center;
		gap: 3rem;
		padding: clamp(2rem, 4vw, 3.5rem) clamp(1rem, 5vw, 5rem);
		background: linear-gradient(
			135deg,
			color-mix(in oklch, var(--periwinkle) 34%, var(--deep)) 0 46%,
			var(--deep) 46%
		);
	}
	.hero .eyebrow {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--sun);
	}
	.hero-copy {
		max-width: 37rem;
		margin: 1.4rem 0 0;
		color: var(--muted);
		font-size: 1.05rem;
	}
	.hero-tools {
		display: grid;
		gap: 0.7rem;
		align-content: end;
	}
	.search-cta {
		display: flex;
		min-height: 4.25rem;
		align-items: center;
		gap: 0.75rem;
		border-radius: 1rem;
		background: var(--aqua);
		padding: 1rem 1.1rem;
		color: var(--ink);
		font-weight: 800;
	}
	.search-cta span {
		flex: 1;
	}
	.rails {
		display: grid;
		width: 100%;
		min-width: 0;
		gap: 3rem;
		padding: 3rem 0;
	}
	.loading-rail {
		display: flex;
		width: 100%;
		min-width: 0;
		box-sizing: border-box;
		gap: 1rem;
		overflow-x: auto;
		overflow-y: hidden;
		padding: 3rem 5vw;
	}
	.loading-rail > div {
		min-width: 20rem;
		height: 16rem;
		border-radius: 1.2rem;
	}
	@media (max-width: 760px) {
		.hero {
			min-height: 0;
			grid-template-columns: 1fr;
			align-content: initial;
			gap: 1.5rem;
			padding-block: clamp(2rem, 9vw, 3rem);
			background: linear-gradient(
				155deg,
				color-mix(in oklch, var(--periwinkle) 38%, var(--deep)) 0 57%,
				var(--deep) 57%
			);
		}
		.hero-tools {
			align-content: initial;
		}
	}
</style>
