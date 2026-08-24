<script lang="ts">
	import { createVirtualizer } from '@tanstack/svelte-virtual';
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { goto } from '$app/navigation';
	import { Play } from '@lucide/svelte';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import type { GuideChannel, Programme } from '$lib/api/types';

	let { channels, from, to } = $props<{ channels: GuideChannel[]; from: Date; to: Date }>();
	let viewport: HTMLDivElement;
	const hourWidth = 220;
	let timelineWidth = $derived(
		Math.max(880, ((to.getTime() - from.getTime()) / 3_600_000) * hourWidth)
	);
	let marks = $derived.by(() => {
		const items: Date[] = [];
		const value = new Date(from);
		value.setMinutes(Math.ceil(value.getMinutes() / 30) * 30, 0, 0);
		while (value < to) {
			items.push(new Date(value));
			value.setMinutes(value.getMinutes() + 30);
		}
		return items;
	});
	const rowVirtualizer = createVirtualizer<HTMLDivElement, HTMLDivElement>({
		count: 0,
		getScrollElement: () => viewport,
		estimateSize: () => 96,
		overscan: 7
	});
	onMount(() => {
		const instance = get(rowVirtualizer);
		instance.setOptions({ count: channels.length, getScrollElement: () => viewport });
		instance.measure();
		if (nowX >= 0 && nowX <= timelineWidth) viewport.scrollLeft = Math.max(0, nowX - 320);
	});
	let nowX = $derived(
		((Date.now() - from.getTime()) / (to.getTime() - from.getTime())) * timelineWidth
	);
	let activeRow = $state(0),
		activeProgramme = $state(0);
	function left(programme: Programme) {
		return Math.max(
			0,
			((new Date(programme.start).getTime() - from.getTime()) / (to.getTime() - from.getTime())) *
				timelineWidth
		);
	}
	function width(programme: Programme) {
		return Math.max(
			54,
			((new Date(programme.end).getTime() - new Date(programme.start).getTime()) /
				(to.getTime() - from.getTime())) *
				timelineWidth -
				3
		);
	}
	function time(value: string | Date) {
		return new Intl.DateTimeFormat([], { hour: 'numeric', minute: '2-digit' }).format(
			new Date(value)
		);
	}
	function focus(row: number, programme: number) {
		activeRow = Math.max(0, Math.min(channels.length - 1, row));
		activeProgramme = Math.max(
			0,
			Math.min((channels[activeRow]?.programmes.length ?? 1) - 1, programme)
		);
		requestAnimationFrame(() =>
			(
				document.querySelector(
					`[data-guide-cell="${activeRow}-${activeProgramme}"]`
				) as HTMLElement | null
			)?.focus()
		);
	}
	function keys(event: KeyboardEvent, row: number, programme: number) {
		if (event.key === 'ArrowRight') {
			event.preventDefault();
			focus(row, programme + 1);
		} else if (event.key === 'ArrowLeft') {
			event.preventDefault();
			focus(row, programme - 1);
		} else if (event.key === 'ArrowDown') {
			event.preventDefault();
			focus(row + 1, programme);
		} else if (event.key === 'ArrowUp') {
			event.preventDefault();
			focus(row - 1, programme);
		} else if (event.key === 'Home') {
			event.preventDefault();
			focus(row, 0);
		} else if (event.key === 'End') {
			event.preventDefault();
			focus(row, Math.max(0, channels[row].programmes.length - 1));
		} else if (event.key === 'PageDown') {
			event.preventDefault();
			viewport.scrollBy({ top: viewport.clientHeight - 96, behavior: 'smooth' });
		} else if (event.key === 'PageUp') {
			event.preventDefault();
			viewport.scrollBy({ top: -viewport.clientHeight + 96, behavior: 'smooth' });
		}
	}
</script>

<div
	class="guide-grid"
	bind:this={viewport}
	role="grid"
	aria-label="Live programme guide"
	aria-rowcount={channels.length}
>
	<div class="ruler" style={`width:${timelineWidth + 196}px`}>
		<div class="ruler-channel">Channel</div>
		<div class="ruler-time" style={`width:${timelineWidth}px`}>
			{#each marks as mark}<span
					style={`left:${((mark.getTime() - from.getTime()) / (to.getTime() - from.getTime())) * timelineWidth}px`}
					>{time(mark)}</span
				>{/each}
		</div>
	</div>
	<div
		class="virtual-space"
		style={`height:${$rowVirtualizer.getTotalSize() + 52}px;width:${timelineWidth + 196}px`}
	>
		{#each $rowVirtualizer.getVirtualItems() as virtualRow (virtualRow.key)}
			{@const channel = channels[virtualRow.index]}
			<div
				class="guide-row"
				role="row"
				aria-rowindex={virtualRow.index + 1}
				data-index={virtualRow.index}
				style={`transform:translateY(${virtualRow.start + 52}px);width:${timelineWidth + 196}px`}
			>
				<div class="channel-cell" role="rowheader">
					<LogoTile src={channel.logo_url} name={channel.name} size="sm" unbounded /><span
						><b>{channel.name}</b><small class="tabular"
							>{channel.number} · {channel.group_name}</small
						></span
					><a href={`/watch/channel/${channel.id}`} aria-label={`Play ${channel.name}`}
						><Play size={15} fill="currentColor" /></a
					>
				</div>
				<div class="programme-track" style={`width:${timelineWidth}px`}>
					{#if channel.programmes.length}
						{#each channel.programmes as programme, programmeIndex}
							<button
								class:current={channel.current?.id === programme.id}
								role="gridcell"
								tabindex={activeRow === virtualRow.index && activeProgramme === programmeIndex
									? 0
									: -1}
								data-guide-cell={`${virtualRow.index}-${programmeIndex}`}
								style={`left:${left(programme)}px;width:${width(programme)}px`}
								onclick={() => goto(`/watch/channel/${channel.id}`)}
								onkeydown={(event) => keys(event, virtualRow.index, programmeIndex)}
								title={`${programme.title}, ${time(programme.start)} to ${time(programme.end)}`}
								><strong>{programme.title}</strong><small class="tabular"
									>{time(programme.start)}–{time(programme.end)}</small
								></button
							>
						{/each}
					{:else}<a class="unavailable" href={`/watch/channel/${channel.id}`}
							>Schedule unavailable · Play channel</a
						>{/if}
					{#if nowX >= 0 && nowX <= timelineWidth}<i class="now-line" style={`left:${nowX}px`}
							><span>Now</span></i
						>{/if}
				</div>
			</div>
		{/each}
	</div>
</div>

<div class="agenda" aria-label="Live programme agenda">
	{#each channels as channel}<section>
			<header>
				<LogoTile src={channel.logo_url} name={channel.name} size="sm" unbounded />
				<div><b>{channel.name}</b><small>{channel.number} · {channel.group_name}</small></div>
				<a href={`/watch/channel/${channel.id}`}><Play size={17} fill="currentColor" />Play</a>
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
		</section>{/each}
</div>

<style>
	.guide-grid {
		position: relative;
		height: calc(100dvh - 13.5rem);
		min-height: 30rem;
		overflow: auto;
		overscroll-behavior: contain;
		background: var(--deep);
	}
	.ruler {
		position: sticky;
		z-index: 20;
		top: 0;
		display: flex;
		height: 52px;
		border-bottom: 1px solid var(--line);
		background: var(--watch-card);
	}
	.ruler-channel {
		position: sticky;
		z-index: 22;
		left: 0;
		display: flex;
		width: 196px;
		flex: none;
		align-items: center;
		border-right: 1px solid var(--line);
		background: var(--watch-card);
		padding: 0.75rem;
		color: var(--muted);
		font-size: 0.7rem;
		font-weight: 800;
		text-transform: uppercase;
	}
	.ruler-time {
		position: relative;
	}
	.ruler-time span {
		position: absolute;
		top: 0;
		height: 100%;
		border-left: 1px solid var(--line);
		padding: 0.85rem 0.65rem;
		color: var(--muted);
		font-size: 0.72rem;
		white-space: nowrap;
	}
	.virtual-space {
		position: relative;
	}
	.guide-row {
		position: absolute;
		top: 0;
		left: 0;
		display: flex;
		height: 96px;
		border-bottom: 1px solid var(--line);
	}
	.channel-cell {
		position: sticky;
		z-index: 12;
		left: 0;
		display: flex;
		width: 196px;
		flex: none;
		align-items: center;
		gap: 0.55rem;
		border-right: 1px solid var(--line);
		background: var(--watch-card);
		padding: 0.65rem;
	}
	.channel-cell > span {
		display: grid;
		min-width: 0;
		flex: 1;
	}
	.channel-cell b {
		overflow: hidden;
		font-size: 0.75rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.channel-cell small {
		color: var(--muted);
		font-size: 0.63rem;
	}
	.channel-cell a {
		display: grid;
		width: 2.25rem;
		height: 2.25rem;
		place-items: center;
		border-radius: 50%;
		background: var(--coral);
		color: var(--ink);
	}
	.programme-track {
		position: relative;
		height: 100%;
		background: repeating-linear-gradient(
			90deg,
			transparent 0,
			transparent 109px,
			var(--line) 110px
		);
	}
	.programme-track button {
		position: absolute;
		top: 6px;
		bottom: 6px;
		overflow: hidden;
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		background: var(--surface);
		padding: 0.55rem 0.65rem;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}
	.programme-track button:hover,
	.programme-track button:focus-visible {
		z-index: 4;
		border-color: var(--aqua);
		background: var(--surface-raised);
	}
	.programme-track button.current {
		border-color: color-mix(in oklch, var(--aqua) 50%, var(--line));
		background: color-mix(in oklch, var(--aqua) 12%, var(--surface));
	}
	.programme-track button strong,
	.programme-track button small {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.programme-track button strong {
		font-size: 0.75rem;
	}
	.programme-track button small {
		margin-top: 0.25rem;
		color: var(--muted);
		font-size: 0.62rem;
	}
	.unavailable {
		position: absolute;
		top: 17px;
		left: 1rem;
		display: flex;
		height: 60px;
		align-items: center;
		border: 1px dashed var(--line);
		border-radius: 0.65rem;
		padding: 0 1rem;
		color: var(--muted);
		font-size: 0.72rem;
	}
	.now-line {
		position: absolute;
		z-index: 8;
		top: 0;
		bottom: 0;
		width: 2px;
		background: var(--coral);
		pointer-events: none;
	}
	.now-line span {
		position: absolute;
		top: 2px;
		left: 5px;
		border-radius: 0.3rem;
		background: var(--coral);
		padding: 0.1rem 0.3rem;
		color: var(--ink);
		font-size: 0.58rem;
		font-weight: 850;
	}
	.agenda {
		display: none;
	}
	.agenda section {
		border-bottom: 1px solid var(--line);
		background: var(--watch-card);
		padding: 1rem;
	}
	.agenda header {
		display: flex;
		align-items: center;
		gap: 0.65rem;
	}
	.agenda header > div {
		display: grid;
		min-width: 0;
		flex: 1;
	}
	.agenda header small {
		color: var(--muted);
		font-size: 0.7rem;
	}
	.agenda header > a {
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
		grid-template-columns: 4.6rem 1fr;
		gap: 0.6rem;
		border-left: 2px solid var(--line);
		padding: 0.65rem 0.8rem;
		color: var(--muted);
		font-size: 0.78rem;
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
	@media (max-width: 700px) {
		.guide-grid {
			display: none;
		}
		.agenda {
			display: block;
		}
	}
</style>
