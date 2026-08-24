<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { Search, ArrowRight, Sparkles } from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { GuideChannel, LineupSummary, Paginated } from '$lib/api/types';
	import { preferences, selectLineup } from '$lib/state/preferences.svelte';
	import LineupPicker from '$lib/components/watch/LineupPicker.svelte';
	import ChannelCard from '$lib/components/watch/ChannelCard.svelte';
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
	const channelsQuery = createQuery(() => ({
		queryKey: ['watch', 'home', lineupId],
		enabled: !!lineupId,
		queryFn: () =>
			api<Paginated<GuideChannel>>(
				`/api/v2/watch/lineups/${lineupId}/channels${params({ limit: 240 })}`
			)
	}));
	let groups = $derived.by(() => {
		const result = new Map<string, GuideChannel[]>();
		for (const channel of channelsQuery.data?.items ?? [])
			result.set(channel.group_name, [...(result.get(channel.group_name) ?? []), channel]);
		return [...result.entries()];
	});
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
			<p class="eyebrow"><Sparkles size={13} /> Your television, organized</p>
			<h1 class="page-title">What’s good<br />right now?</h1>
			<p class="hero-copy">
				Jump into something live, or open the guide when you want the full picture.
			</p>
		</div>
		<div class="hero-tools">
			{#if lineupsQuery.data}<LineupPicker
					items={lineupsQuery.data.items}
					value={lineupId}
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
	{:else if channelsQuery.isError}<EmptyState
			title="We lost the signal"
			message="The lineup could not be loaded. Your streams and published outputs are unaffected."
			action="Try again"
			href="/"
		/>
	{:else if channelsQuery.isPending}<div class="loading-rail">
			{#each Array(4) as _}<div class="skeleton"></div>{/each}
		</div>
	{:else if !channelsQuery.data?.total}<EmptyState
			title="This lineup is quiet"
			message="Add playable channels in Studio, then come back here to watch."
			action="Edit lineup"
			href="/studio/lineups"
		/>
	{:else}<div class="rails">
			{#each groups as [name, channels]}<section class="rail">
					<header>
						<div>
							<p class="eyebrow">On now</p>
							<h2>{name}</h2>
						</div>
						<a href={`/channels?group=${encodeURIComponent(name)}`}
							>See all <ArrowRight size={16} /></a
						>
					</header>
					<div class="rail-scroll">
						{#each channels as channel}<ChannelCard {channel} />{/each}
					</div>
				</section>{/each}
		</div>{/if}
</div>

<style>
	.home-page {
		padding-bottom: 4rem;
	}
	.hero {
		display: grid;
		min-height: 29rem;
		grid-template-columns: 1.25fr 0.75fr;
		align-items: end;
		gap: 3rem;
		padding: clamp(3rem, 8vw, 7rem) clamp(1rem, 5vw, 5rem) 3rem;
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
		gap: 3rem;
		padding: 3rem 0;
	}
	.rail header {
		display: flex;
		align-items: end;
		justify-content: space-between;
		padding: 0 clamp(1rem, 5vw, 5rem) 1rem;
	}
	.rail h2 {
		margin: 0;
		font-size: clamp(1.6rem, 3vw, 2.45rem);
	}
	.rail header a {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--aqua);
		font-size: 0.84rem;
		font-weight: 800;
	}
	.rail-scroll {
		display: flex;
		gap: 1rem;
		overflow-x: auto;
		padding: 0.35rem clamp(1rem, 5vw, 5rem) 1.5rem;
		scroll-padding-inline: clamp(1rem, 5vw, 5rem);
		scroll-snap-type: x proximity;
		scrollbar-width: thin;
	}
	.loading-rail {
		display: flex;
		gap: 1rem;
		overflow: hidden;
		padding: 3rem 5vw;
	}
	.loading-rail > div {
		min-width: 20rem;
		height: 16rem;
		border-radius: 1.2rem;
	}
	@media (max-width: 760px) {
		.hero {
			min-height: 32rem;
			grid-template-columns: 1fr;
			align-content: end;
			gap: 2rem;
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
