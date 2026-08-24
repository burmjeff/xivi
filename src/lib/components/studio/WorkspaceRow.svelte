<script lang="ts">
	import { onMount } from 'svelte';
	import {
		draggable,
		dropTargetForElements
	} from '@atlaskit/pragmatic-drag-and-drop/adapter/element-adapter';
	import { combine } from '@atlaskit/pragmatic-drag-and-drop/utils/combine';
	import { announce } from '@atlaskit/pragmatic-drag-and-drop-live-region';
	import {
		GripVertical,
		ChevronUp,
		ChevronDown,
		ChevronsUp,
		ChevronsDown,
		Trash2,
		LockKeyhole,
		CircleAlert,
		CheckCircle2
	} from '@lucide/svelte';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import type { WorkspaceChannel } from '$lib/api/types';
	let {
		channel,
		selected = false,
		checked = false,
		previousId,
		nextId,
		firstId,
		lastId,
		onselect,
		ontoggle,
		onmove,
		onremove
	} = $props<{
		channel: WorkspaceChannel;
		selected?: boolean;
		checked?: boolean;
		previousId?: number;
		nextId?: number;
		firstId?: number;
		lastId?: number;
		onselect: () => void;
		ontoggle: () => void;
		onmove: (sourceId: number, targetId: number, placement: 'before' | 'after') => void;
		onremove: () => void;
	}>();
	let row: HTMLElement,
		handle: HTMLElement,
		dragging = $state(false),
		over = $state(false);
	onMount(() =>
		combine(
			draggable({
				element: row,
				dragHandle: handle,
				getInitialData: () => ({ channelId: channel.id, name: channel.name }),
				onDragStart: () => {
					dragging = true;
					announce(`Picked up ${channel.name}`);
				},
				onDrop: () => (dragging = false)
			}),
			dropTargetForElements({
				element: row,
				getData: () => ({ targetId: channel.id }),
				onDragEnter: ({ source }) => {
					if (Number(source.data.channelId) !== channel.id) over = true;
				},
				onDragLeave: () => (over = false),
				onDrop: ({ source }) => {
					over = false;
					const sourceId = Number(source.data.channelId);
					if (sourceId !== channel.id) {
						onmove(sourceId, channel.id, 'before');
						announce(`Moved ${source.data.name} before ${channel.name}`);
					}
				}
			})
		)
	);
	let health = $derived(
		channel.source_count === 0
			? 'missing'
			: channel.match_score !== undefined && channel.match_score < 0.82 && !channel.manual_locked
				? 'review'
				: 'good'
	);
</script>

<article bind:this={row} class:selected class:dragging class:over>
	<button
		bind:this={handle}
		class="drag-handle"
		aria-label={`Drag ${channel.name} to reorder`}
		title="Drag to reorder"><GripVertical size={17} /></button
	><label class="row-check"
		><input type="checkbox" {checked} onchange={ontoggle} /><span class="sr-only"
			>Select {channel.name}</span
		></label
	><button class="row-main" onclick={onselect}
		><LogoTile src={channel.logo_url} name={channel.name} size="sm" /><span class="row-copy"
			><strong>{channel.name}</strong><small
				>{channel.tvg_id || 'No TVG ID'} · {channel.source_count} source{channel.source_count === 1
					? ''
					: 's'}</small
			></span
		></button
	><span
		class="match-health {health}"
		title={health === 'good'
			? 'Match healthy'
			: health === 'review'
				? 'Review match'
				: 'No source variant'}
		>{#if channel.manual_locked}<LockKeyhole size={15} />{:else if health === 'good'}<CheckCircle2
				size={16}
			/>{:else}<CircleAlert size={16} />{/if}</span
	>
	<div class="move-buttons">
		<button
			disabled={!previousId || !firstId}
			onclick={() => {
				if (firstId) onmove(channel.id, firstId, 'before');
			}}
			aria-label={`Move ${channel.name} to top`}><ChevronsUp size={16} /></button
		>
		<button
			disabled={!previousId}
			onclick={() => {
				if (previousId) onmove(channel.id, previousId, 'before');
			}}
			aria-label={`Move ${channel.name} up`}><ChevronUp size={16} /></button
		><button
			disabled={!nextId}
			onclick={() => {
				if (nextId) onmove(channel.id, nextId, 'after');
			}}
			aria-label={`Move ${channel.name} down`}><ChevronDown size={16} /></button
		><button
			disabled={!nextId || !lastId}
			onclick={() => {
				if (lastId) onmove(channel.id, lastId, 'after');
			}}
			aria-label={`Move ${channel.name} to bottom`}><ChevronsDown size={16} /></button
		><button onclick={onremove} aria-label={`Remove ${channel.name}`}><Trash2 size={16} /></button>
	</div>
</article>

<style>
	article {
		display: grid;
		grid-template-columns: auto auto minmax(0, 1fr) auto auto;
		align-items: center;
		gap: 0.55rem;
		min-height: 4rem;
		border-bottom: 1px solid var(--line);
		padding: 0.45rem 0.55rem;
		transition:
			background var(--micro),
			opacity var(--micro);
	}
	article:hover,
	article.selected {
		background: color-mix(in oklch, var(--periwinkle) 11%, var(--surface));
	}
	article.over {
		box-shadow: inset 0 3px var(--aqua);
	}
	article.dragging {
		opacity: 0.45;
	}
	.drag-handle,
	.move-buttons button {
		display: grid;
		width: 2rem;
		height: 2rem;
		place-items: center;
		border: 0;
		border-radius: 0.45rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.drag-handle {
		cursor: grab;
	}
	.row-check {
		display: grid;
		min-width: 1.5rem;
		min-height: 1.5rem;
		place-items: center;
	}
	.row-check input {
		width: 1rem;
		height: 1rem;
		accent-color: var(--periwinkle);
	}
	.row-main {
		display: grid;
		min-width: 0;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: center;
		gap: 0.55rem;
		border: 0;
		background: transparent;
		padding: 0;
		color: inherit;
		text-align: left;
		cursor: pointer;
	}
	.row-copy {
		display: grid;
		min-width: 0;
	}
	.row-copy strong,
	.row-copy small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.row-copy strong {
		font-size: 0.77rem;
	}
	.row-copy small {
		color: var(--muted);
		font-size: 0.63rem;
	}
	.match-health {
		display: grid;
		width: 1.7rem;
		height: 1.7rem;
		place-items: center;
		border-radius: 50%;
	}
	.match-health.good {
		color: var(--success);
	}
	.match-health.review {
		color: var(--sun);
	}
	.match-health.missing {
		color: var(--error);
	}
	.move-buttons {
		display: flex;
	}
	.move-buttons button:hover {
		background: var(--surface-raised);
		color: var(--text);
	}
	.move-buttons button:disabled {
		opacity: 0.25;
		cursor: not-allowed;
	}
	@media (max-width: 600px) {
		article {
			grid-template-columns: auto minmax(0, 1fr) auto;
		}
		.drag-handle,
		.match-health {
			display: none;
		}
		.move-buttons button:last-child {
			display: none;
		}
	}
</style>
