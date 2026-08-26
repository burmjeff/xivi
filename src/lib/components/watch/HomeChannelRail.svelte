<script lang="ts">
	import { createInfiniteQuery } from '@tanstack/svelte-query';
	import { createVirtualizer, type SvelteVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { onMount } from 'svelte';
	import { ArrowRight, ChevronLeft, ChevronRight, LoaderCircle } from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { GuideChannel, Paginated, WatchGroupSummary } from '$lib/api/types';
	import ChannelCard from './ChannelCard.svelte';

	let {
		lineupId,
		group,
		eager = false
	} = $props<{ lineupId: number; group: WatchGroupSummary; eager?: boolean }>();
	let section: HTMLElement;
	let viewport = $state<HTMLDivElement>();
	let visible = $state(false);

	onMount(() => {
		virtualizerInstance = get(railVirtualizer);
		configureVirtualizer();
		visible = eager;
		if (visible || !('IntersectionObserver' in window)) {
			visible = true;
			return;
		}
		const observer = new IntersectionObserver(
			(entries) => {
				if (!entries.some((entry) => entry.isIntersecting)) return;
				visible = true;
				observer.disconnect();
			},
			{ rootMargin: '600px 0px' }
		);
		observer.observe(section);
		return () => observer.disconnect();
	});

	const channelsQuery = createInfiniteQuery(() => ({
		queryKey: ['watch', 'home-rail', lineupId, group.id],
		enabled: visible,
		initialPageParam: '' as string,
		queryFn: ({ pageParam }) =>
			api<Paginated<GuideChannel>>(
				`/api/v2/watch/lineups/${lineupId}/channels${params({
					group_id: group.id,
					cursor: typeof pageParam === 'string' && pageParam ? pageParam : undefined,
					limit: 24
				})}`
			),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined
	}));
	let channels = $derived(channelsQuery.data?.pages.flatMap((page) => page.items) ?? []);
	let itemCount = $derived(channels.length + (channelsQuery.hasNextPage ? 1 : 0));
	const railVirtualizer = createVirtualizer<HTMLDivElement, HTMLDivElement>({
		count: 0,
		getScrollElement: () => viewport ?? null,
		estimateSize: () => 328,
		horizontal: true,
		overscan: 3
	});
	let virtualizerInstance: SvelteVirtualizer<HTMLDivElement, HTMLDivElement> | undefined;
	function configureVirtualizer() {
		if (!virtualizerInstance) return;
		virtualizerInstance.setOptions({
			count: itemCount,
			getScrollElement: () => viewport ?? null,
			estimateSize: () => 328,
			horizontal: true,
			overscan: 3
		});
		virtualizerInstance.measure();
	}
	$effect(() => {
		itemCount;
		viewport;
		configureVirtualizer();
	});
	$effect(() => {
		const last = $railVirtualizer.getVirtualItems().at(-1);
		if (
			last &&
			last.index >= channels.length - 3 &&
			channelsQuery.hasNextPage &&
			!channelsQuery.isFetchingNextPage
		) {
			void channelsQuery.fetchNextPage();
		}
	});

	function scroll(direction: -1 | 1) {
		viewport?.scrollBy({
			left: direction * Math.max(viewport.clientWidth * 0.82, 320),
			behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
		});
	}
</script>

<section class="rail" bind:this={section}>
	<header>
		<div>
			<p class="eyebrow">On now</p>
			<h2>{group.name}</h2>
		</div>
		<div class="rail-actions">
			<div class="rail-nav" aria-label={`Scroll ${group.name} channels`}>
				<button aria-label={`Scroll ${group.name} left`} onclick={() => scroll(-1)}
					><ChevronLeft size={18} /></button
				><button aria-label={`Scroll ${group.name} right`} onclick={() => scroll(1)}
					><ChevronRight size={18} /></button
				>
			</div>
			<a href={`/channels?group=${group.id}`}>See all <ArrowRight size={16} /></a>
		</div>
	</header>
	{#if !visible || channelsQuery.isPending}
		<div class="loading-rail" aria-label={`Loading ${group.name} channels`}>
			{#each Array(4) as _}<div class="skeleton"></div>{/each}
		</div>
	{:else if channelsQuery.isError}
		<div class="rail-status">This group could not be loaded.</div>
	{:else}
		<!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users need to scroll the rail) -->
		<div
			class="rail-scroll"
			bind:this={viewport}
			role="region"
			aria-label={`${group.name} channels, horizontal list`}
			tabindex="0"
		>
			<div class="rail-space" style={`width:${$railVirtualizer.getTotalSize()}px`}>
				{#each $railVirtualizer.getVirtualItems() as virtualItem (virtualItem.key)}
					<div
						class="rail-item"
						style={`width:${virtualItem.size - 16}px;transform:translateX(${virtualItem.start}px)`}
					>
						{#if virtualItem.index < channels.length}
							<ChannelCard channel={channels[virtualItem.index]} />
						{:else}
							<button class="load-more" onclick={() => channelsQuery.fetchNextPage()}>
								{#if channelsQuery.isFetchingNextPage}<LoaderCircle class="spin" size={18} />{/if}
								{channelsQuery.isFetchingNextPage ? 'Loading…' : 'Load more'}
							</button>
						{/if}
					</div>
				{/each}
			</div>
		</div>
	{/if}
</section>

<style>
	.rail {
		width: 100%;
		min-width: 0;
	}
	header {
		display: flex;
		align-items: end;
		justify-content: space-between;
		padding: 0 clamp(1rem, 5vw, 5rem) 1rem;
	}
	h2 {
		margin: 0;
		font-size: clamp(1.6rem, 3vw, 2.45rem);
	}
	.rail-actions,
	.rail-nav {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.rail-nav {
		gap: 0.3rem;
	}
	.rail-nav button {
		display: grid;
		width: 2.5rem;
		height: 2.5rem;
		place-items: center;
		border: 1px solid var(--line);
		border-radius: 999px;
		background: var(--watch-card);
		color: var(--text);
		cursor: pointer;
	}
	.rail-nav button:hover {
		border-color: color-mix(in oklch, var(--aqua) 55%, var(--line));
		background: color-mix(in oklch, var(--aqua) 12%, var(--watch-card));
	}
	header a {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--aqua);
		font-size: 0.84rem;
		font-weight: 800;
	}
	.rail-scroll,
	.loading-rail {
		width: 100%;
		min-width: 0;
		box-sizing: border-box;
		overflow-x: auto;
		overflow-y: hidden;
		padding: 0.35rem clamp(1rem, 5vw, 5rem) 1.5rem;
		scrollbar-width: thin;
		scrollbar-color: color-mix(in oklch, var(--aqua) 50%, var(--line)) transparent;
	}
	.rail-scroll {
		overscroll-behavior-inline: contain;
	}
	.rail-scroll:focus-visible {
		outline: 2px solid var(--aqua);
		outline-offset: -2px;
	}
	.rail-space {
		position: relative;
		height: 17.1rem;
	}
	.rail-item {
		position: absolute;
		top: 0;
		left: 0;
	}
	.rail-item :global(.channel-card) {
		width: 100%;
		min-width: 0;
		max-width: none;
		box-sizing: border-box;
	}
	.loading-rail {
		display: flex;
		gap: 1rem;
	}
	.loading-rail > div {
		min-width: 19.5rem;
		height: 16rem;
		border-radius: 1.2rem;
	}
	.load-more {
		display: flex;
		width: 100%;
		height: 16rem;
		align-items: center;
		justify-content: center;
		gap: 0.45rem;
		border: 1px dashed var(--line);
		border-radius: 1.2rem;
		background: var(--watch-card);
		color: var(--aqua);
		font-weight: 800;
		cursor: pointer;
	}
	.rail-status {
		margin: 0 clamp(1rem, 5vw, 5rem);
		border: 1px dashed var(--line);
		border-radius: 1rem;
		padding: 2rem;
		color: var(--muted);
	}
	:global(.spin) {
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (max-width: 760px) {
		header {
			align-items: center;
		}
		.rail-nav button {
			width: 2.75rem;
			height: 2.75rem;
		}
		header a {
			display: none;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		:global(.spin) {
			animation: none;
		}
	}
</style>
