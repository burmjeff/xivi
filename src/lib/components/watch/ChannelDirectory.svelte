<script lang="ts">
	import { createVirtualizer, type SvelteVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { onMount } from 'svelte';
	import { ArrowRight, LoaderCircle, Play } from '@lucide/svelte';
	import type { GuideChannel } from '$lib/api/types';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';

	let { channels, total, view, hasNextPage, isFetchingNextPage, onLoadMore } = $props<{
		channels: GuideChannel[];
		total: number;
		view: 'grid' | 'list';
		hasNextPage: boolean;
		isFetchingNextPage: boolean;
		onLoadMore: () => void;
	}>();
	let viewport: HTMLDivElement;
	let viewportWidth = $state(960);
	let columns = $derived(view === 'grid' ? Math.max(1, Math.floor((viewportWidth + 16) / 304)) : 1);
	let channelRowCount = $derived(Math.ceil(channels.length / columns));
	let rowCount = $derived(channelRowCount + (hasNextPage ? 1 : 0));
	const virtualizer = createVirtualizer<HTMLDivElement, HTMLDivElement>({
		count: 0,
		getScrollElement: () => viewport,
		estimateSize: () => (view === 'grid' ? 384 : 104),
		overscan: 3
	});
	let virtualizerInstance: SvelteVirtualizer<HTMLDivElement, HTMLDivElement> | undefined;

	function configureVirtualizer() {
		if (!virtualizerInstance) return;
		const nextRowHeight = view === 'grid' ? 384 : viewportWidth <= 800 ? 124 : 104;
		virtualizerInstance.setOptions({
			count: rowCount,
			getScrollElement: () => viewport,
			estimateSize: () => nextRowHeight,
			overscan: 3
		});
		virtualizerInstance.measurementsCache = [];
		virtualizerInstance.measure();
	}
	onMount(() => {
		virtualizerInstance = get(virtualizer);
		configureVirtualizer();
		const observer = new ResizeObserver(([entry]) => {
			viewportWidth = entry.contentRect.width;
		});
		observer.observe(viewport);
		return () => observer.disconnect();
	});
	$effect(() => {
		view;
		rowCount;
		viewport;
		viewportWidth;
		configureVirtualizer();
	});
	$effect(() => {
		const last = $virtualizer.getVirtualItems().at(-1);
		if (last && last.index >= channelRowCount - 2 && hasNextPage && !isFetchingNextPage) {
			onLoadMore();
		}
	});

	function time(value?: string) {
		return value
			? new Intl.DateTimeFormat([], { hour: 'numeric', minute: '2-digit' }).format(new Date(value))
			: '';
	}
	function programmeProgress(channel: GuideChannel) {
		if (!channel.current) return 0;
		const start = new Date(channel.current.start).getTime();
		const end = new Date(channel.current.end).getTime();
		if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return 0;
		return Math.max(0, Math.min(100, ((Date.now() - start) / (end - start)) * 100));
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users need to scroll the virtual directory) -->
<div
	class:grid={view === 'grid'}
	class:list={view === 'list'}
	class="directory-viewport"
	bind:this={viewport}
	role="feed"
	aria-label="Channel directory"
	aria-busy={isFetchingNextPage}
	tabindex="0"
>
	<div class="virtual-space" style={`height:${$virtualizer.getTotalSize()}px`}>
		{#each $virtualizer.getVirtualItems() as virtualRow (virtualRow.key)}
			<div
				class="virtual-row"
				style={`height:${virtualRow.size}px;transform:translateY(${virtualRow.start}px);grid-template-columns:repeat(${columns},minmax(0,1fr))`}
			>
				{#if virtualRow.index < channelRowCount}
					{#each channels.slice(virtualRow.index * columns, (virtualRow.index + 1) * columns) as channel, columnIndex (channel.id)}
						{@const progress = programmeProgress(channel)}
						<article
							aria-posinset={virtualRow.index * columns + columnIndex + 1}
							aria-setsize={total}
						>
							<span class="live-pill">Live</span>
							<LogoTile
								src={channel.logo_url}
								name={channel.name}
								size={view === 'grid' ? 'lg' : 'md'}
								unbounded
							/>
							<div class="channel-copy">
								<span class="channel-meta tabular">{channel.number} · {channel.group_name}</span>
								<h2>{channel.name}</h2>
								<div class="now">
									<strong>{channel.current?.title ?? 'Schedule unavailable'}</strong>
									<div class="programme-timeline">
										{#if channel.current}<time
												>{time(channel.current.start)}–{time(channel.current.end)}</time
											>{/if}
										<div
											class="programme-progress"
											role="progressbar"
											aria-valuemin="0"
											aria-valuemax="100"
											aria-valuenow={Math.round(progress)}
											aria-label={channel.current
												? `${channel.current.title}, ${Math.round(progress)} percent complete`
												: 'Schedule unavailable'}
										>
											<i style={`width:${progress}%`}></i>
										</div>
									</div>
								</div>
								{#if channel.next}<p>
										Next at {time(channel.next.start)} <b>{channel.next.title}</b>
									</p>{:else}<p>Upcoming schedule unavailable</p>{/if}
							</div>
							<a
								class="play"
								href={`/watch/channel/${channel.id}`}
								aria-label={`Play ${channel.name}`}
								><Play size={20} fill="currentColor" /><span>Play</span><ArrowRight size={17} /></a
							>
						</article>
					{/each}
				{:else}
					<button class="load-more" onclick={onLoadMore}>
						{#if isFetchingNextPage}<LoaderCircle class="spin" size={18} />{/if}
						{isFetchingNextPage ? `Loading more of ${total}…` : 'Load more channels'}
					</button>
				{/if}
			</div>
		{/each}
	</div>
</div>

<style>
	.directory-viewport {
		position: relative;
		height: max(30rem, calc(100dvh - 13rem));
		overflow-y: auto;
		overscroll-behavior: contain;
		scrollbar-gutter: stable;
	}
	.virtual-space {
		position: relative;
		width: 100%;
	}
	.virtual-row {
		position: absolute;
		top: 0;
		left: 0;
		display: grid;
		width: 100%;
		gap: 1rem;
		padding-bottom: 1rem;
	}
	article {
		position: relative;
		min-width: 0;
		border: 1px solid var(--watch-card-border);
		background: var(--watch-card);
	}
	.grid article {
		display: grid;
		height: 23rem;
		grid-template-rows: auto 1fr auto;
		justify-items: start;
		border-radius: 1.25rem;
		box-shadow: var(--watch-card-shadow);
		padding: 1.1rem;
		transition:
			transform var(--layout) var(--ease-out),
			box-shadow var(--layout) var(--ease-out);
	}
	.grid article:hover {
		transform: translateY(-3px);
		box-shadow: var(--watch-card-shadow-hover);
	}
	.grid .live-pill {
		position: absolute;
		z-index: 1;
		top: 1.1rem;
		right: 1.1rem;
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
		grid-template-columns: minmax(0, 1fr);
		align-items: center;
		gap: 0.45rem;
	}
	.now strong {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.now time {
		grid-column: 1;
		color: var(--muted);
		font-size: 0.72rem;
	}
	.programme-timeline {
		display: contents;
	}
	.programme-progress {
		grid-column: 1 / -1;
		height: 0.34rem;
		overflow: hidden;
		margin-top: 0.4rem;
		border-radius: 999px;
		background: var(--surface-raised);
	}
	.programme-progress i {
		display: block;
		height: 100%;
		border-radius: inherit;
		background: var(--aqua);
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
	.list .virtual-row {
		display: block;
	}
	.list article {
		display: flex;
		height: 5.95rem;
		align-items: center;
		gap: 1rem;
		border-radius: 1rem;
		padding: 0.75rem;
	}
	.list .channel-copy {
		display: grid;
		grid-template-columns: minmax(10rem, 0.7fr) minmax(15rem, 1.3fr) minmax(12rem, 0.8fr);
		align-items: center;
		gap: 1rem;
	}
	.list .channel-meta {
		margin: 0;
	}
	.list h2 {
		margin: 0.15rem 0 0;
	}
	.list .now {
		grid-row: 1/3;
		grid-column: 2;
	}
	.list .channel-copy > p {
		grid-row: 1/3;
		grid-column: 3;
		margin: 0;
	}
	.list .play {
		width: auto;
		align-self: flex-end;
	}
	.list .live-pill {
		position: absolute;
		z-index: 1;
		top: 0.48rem;
		right: 0.75rem;
		gap: 0.22rem;
		padding: 0.16rem 0.38rem;
		font-size: 0.54rem;
	}
	.list .live-pill::before {
		width: 0.28rem;
		height: 0.28rem;
	}
	.list .play span,
	.list .play :global(svg:last-child) {
		display: none;
	}
	.load-more {
		display: flex;
		width: 100%;
		height: calc(100% - 1rem);
		grid-column: 1 / -1;
		align-items: center;
		justify-content: center;
		gap: 0.45rem;
		border: 1px dashed var(--line);
		border-radius: 1rem;
		background: var(--watch-card);
		color: var(--aqua);
		font-weight: 800;
		cursor: pointer;
	}
	:global(.spin) {
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (max-width: 800px) {
		.list article {
			height: 6.75rem;
			gap: 0.65rem;
			padding: 0.65rem;
		}
		.list .channel-copy {
			display: flex;
			width: auto;
			min-width: 0;
			height: 100%;
			flex: 1 1 0;
			flex-direction: column;
			gap: 0;
			justify-content: center;
			overflow: hidden;
		}
		.list .channel-meta {
			flex-shrink: 0;
			overflow: hidden;
			font-size: 0.64rem;
			line-height: 1.1;
			text-overflow: ellipsis;
			white-space: nowrap;
		}
		.list h2 {
			flex-shrink: 0;
			margin: 0.18rem 0 0.42rem;
			font-size: 1rem;
			line-height: 1.15;
		}
		.list .now {
			display: grid;
			width: 100%;
			align-self: stretch;
			flex-shrink: 0;
			grid-template-columns: minmax(0, 1fr);
			gap: 0.3rem;
		}
		.list .now strong {
			grid-row: 1;
			grid-column: 1;
		}
		.list .programme-timeline {
			display: grid;
			width: 100%;
			min-width: 0;
			grid-column: 1 / -1;
			grid-template-columns: minmax(0, 1fr) 5.65rem;
			align-items: center;
			gap: 0.3rem;
		}
		.list .now time {
			display: block;
			grid-row: 1;
			grid-column: 2;
			align-self: center;
			font-size: 0.59rem;
			font-variant-numeric: tabular-nums;
			line-height: 1;
			text-align: right;
			white-space: nowrap;
		}
		.list .programme-progress {
			grid-row: 1;
			grid-column: 1;
			height: 0.27rem;
			margin-top: 0;
		}
		.list .channel-copy > p {
			display: none;
		}
		.list .play {
			width: 2.75rem;
			min-width: 2.75rem;
			min-height: 2.75rem;
			justify-content: center;
			padding: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.grid article {
			transition: none;
		}
		:global(.spin) {
			animation: none;
		}
	}
</style>
