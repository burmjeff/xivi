<script lang="ts">
	import { createVirtualizer, type SvelteVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { onMount } from 'svelte';
	import { LoaderCircle, Play } from '@lucide/svelte';
	import type { GuideChannel } from '$lib/api/types';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';

	let { channels, total, hasNextPage, isFetchingNextPage, onLoadMore } = $props<{
		channels: GuideChannel[];
		total: number;
		hasNextPage: boolean;
		isFetchingNextPage: boolean;
		onLoadMore: () => void;
	}>();
	let viewport: HTMLDivElement;
	let count = $derived(channels.length + (hasNextPage ? 1 : 0));
	const virtualizer = createVirtualizer<HTMLDivElement, HTMLElement>({
		count: 0,
		getScrollElement: () => viewport,
		estimateSize: () => 272,
		overscan: 4
	});
	let virtualizerInstance: SvelteVirtualizer<HTMLDivElement, HTMLElement> | undefined;
	function configureVirtualizer() {
		if (!virtualizerInstance) return;
		virtualizerInstance.setOptions({
			count,
			getScrollElement: () => viewport,
			estimateSize: () => 272,
			overscan: 4
		});
		virtualizerInstance.measure();
	}
	onMount(() => {
		virtualizerInstance = get(virtualizer);
		configureVirtualizer();
	});
	$effect(() => {
		count;
		viewport;
		configureVirtualizer();
	});
	$effect(() => {
		const last = $virtualizer.getVirtualItems().at(-1);
		if (last && last.index >= channels.length - 3 && hasNextPage && !isFetchingNextPage) {
			onLoadMore();
		}
	});
	function time(value: string) {
		return new Intl.DateTimeFormat([], { hour: 'numeric', minute: '2-digit' }).format(
			new Date(value)
		);
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users need to scroll the virtual agenda) -->
<div
	class="agenda"
	bind:this={viewport}
	role="region"
	aria-label="Live programme agenda"
	aria-busy={isFetchingNextPage}
	tabindex="0"
>
	<div class="virtual-space" style={`height:${$virtualizer.getTotalSize()}px`}>
		{#each $virtualizer.getVirtualItems() as virtualRow (virtualRow.key)}
			<div
				class="virtual-row"
				style={`height:${virtualRow.size}px;transform:translateY(${virtualRow.start}px)`}
			>
				{#if virtualRow.index < channels.length}
					{@const channel = channels[virtualRow.index]}
					<section>
						<header>
							<LogoTile src={channel.logo_url} name={channel.name} size="sm" unbounded />
							<div><b>{channel.name}</b><small>{channel.number} · {channel.group_name}</small></div>
							<a href={`/watch/channel/${channel.id}`}><Play size={17} fill="currentColor" />Play</a
							>
						</header>
						{#if channel.programmes.length}<div class="agenda-programmes">
								{#each channel.programmes.slice(0, 4) as programme}<a
										class:current={channel.current?.id === programme.id}
										href={`/watch/channel/${channel.id}`}
										><time>{time(programme.start)}</time><span>{programme.title}</span></a
									>{/each}
							</div>{:else}<a class="agenda-empty" href={`/watch/channel/${channel.id}`}
								>Schedule unavailable. Tap to play.</a
							>{/if}
					</section>
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
	.agenda {
		height: calc(100dvh - 16rem);
		min-height: 28rem;
		overflow-y: auto;
		background: var(--deep);
	}
	.virtual-space {
		position: relative;
	}
	.virtual-row {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		padding-bottom: 1px;
	}
	section {
		height: 100%;
		overflow: hidden;
		border-bottom: 1px solid var(--watch-card-border);
		background: var(--watch-card);
		padding: 1rem;
	}
	header {
		display: flex;
		align-items: center;
		gap: 0.65rem;
	}
	header > div {
		display: grid;
		min-width: 0;
		flex: 1;
	}
	header b,
	header small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	header small {
		color: var(--muted);
		font-size: 0.7rem;
	}
	header > a {
		display: flex;
		min-height: 2.75rem;
		align-items: center;
		gap: 0.35rem;
		border-radius: 0.75rem;
		background: var(--coral);
		padding: 0.55rem 0.75rem;
		color: var(--ink);
		font-weight: 800;
	}
	.agenda-programmes {
		display: grid;
		margin: 1rem 0 0 2.9rem;
	}
	.agenda-programmes a {
		display: grid;
		grid-template-columns: 4.6rem minmax(0, 1fr);
		gap: 0.6rem;
		border-left: 2px solid var(--line);
		padding: 0.55rem 0.8rem;
		color: var(--muted);
		font-size: 0.78rem;
	}
	.agenda-programmes span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.agenda-programmes a.current {
		border-color: var(--aqua);
		background: color-mix(in oklch, var(--aqua) 9%, transparent);
		color: var(--text);
	}
	.agenda-programmes time {
		font-variant-numeric: tabular-nums;
	}
	.agenda-empty {
		display: block;
		margin: 0.8rem 0 0 2.9rem;
		color: var(--muted);
		font-size: 0.78rem;
	}
	.load-more {
		display: flex;
		width: calc(100% - 2rem);
		height: calc(100% - 2rem);
		align-items: center;
		justify-content: center;
		gap: 0.45rem;
		border: 1px dashed var(--line);
		border-radius: 1rem;
		margin: 1rem;
		background: var(--watch-card);
		color: var(--aqua);
		font-weight: 800;
	}
	:global(.spin) {
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		:global(.spin) {
			animation: none;
		}
	}
</style>
