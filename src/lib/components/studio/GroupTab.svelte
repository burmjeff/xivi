<script lang="ts">
	import { onMount } from 'svelte';
	import {
		draggable,
		dropTargetForElements
	} from '@atlaskit/pragmatic-drag-and-drop/adapter/element-adapter';
	import { combine } from '@atlaskit/pragmatic-drag-and-drop/utils/combine';
	import { announce } from '@atlaskit/pragmatic-drag-and-drop-live-region';
	import { DropdownMenu } from 'bits-ui';
	import {
		ChevronLeft,
		ChevronRight,
		ChevronsLeft,
		ChevronsRight,
		GripVertical,
		MoreHorizontal,
		Pencil,
		RadioTower,
		Trash2
	} from '@lucide/svelte';
	import type { StudioGroup } from '$lib/api/types';

	let {
		group,
		index,
		selected = false,
		previousId,
		nextId,
		firstId,
		lastId,
		onselect,
		onmove,
		onedit,
		onremove
	} = $props<{
		group: StudioGroup;
		index: number;
		selected?: boolean;
		previousId?: number;
		nextId?: number;
		firstId?: number;
		lastId?: number;
		onselect: () => void;
		onmove: (sourceId: number, targetId: number, placement: 'before' | 'after') => void;
		onedit: () => void;
		onremove: () => void;
	}>();

	let tab: HTMLElement,
		handle: HTMLElement,
		dragging = $state(false),
		overPlacement = $state<'before' | 'after' | null>(null);

	onMount(() =>
		combine(
			draggable({
				element: tab,
				dragHandle: handle,
				getInitialData: () => ({
					kind: 'lineup-group',
					groupId: group.id,
					index,
					name: group.name
				}),
				onDragStart: () => {
					dragging = true;
					announce(`Picked up ${group.name}`);
				},
				onDrop: () => (dragging = false)
			}),
			dropTargetForElements({
				element: tab,
				canDrop: ({ source }) => source.data.kind === 'lineup-group',
				onDragEnter: ({ source }) => {
					if (Number(source.data.groupId) === group.id) return;
					overPlacement = Number(source.data.index) < index ? 'after' : 'before';
				},
				onDragLeave: () => (overPlacement = null),
				onDrop: ({ source }) => {
					const sourceId = Number(source.data.groupId);
					const placement = Number(source.data.index) < index ? 'after' : 'before';
					overPlacement = null;
					if (sourceId === group.id) return;
					onmove(sourceId, group.id, placement);
					announce(`Moved ${source.data.name} ${placement} ${group.name}`);
				}
			})
		)
	);
</script>

<div
	bind:this={tab}
	class="group-tab"
	class:selected
	class:dragging
	class:over-before={overPlacement === 'before'}
	class:over-after={overPlacement === 'after'}
>
	<button
		bind:this={handle}
		class="group-drag"
		aria-label={`Drag ${group.name} to reorder groups`}
		title="Drag to reorder groups"><GripVertical size={14} /></button
	>
	<button class="group-main" onclick={onselect}>
		<span>{group.name}</span><strong>{group.channel_count}</strong>{#if group.source_link}<em
				class:issue={group.source_link.status === 'error' ||
					group.source_link.status === 'disconnected'}
				>{group.source_link.status === 'active' ? 'Synced' : group.source_link.status}</em
			>{/if}
	</button>
	<div class="group-menu-slot">
		<DropdownMenu.Root>
			<DropdownMenu.Trigger class="group-menu-trigger" aria-label={`Arrange or edit ${group.name}`}>
				<MoreHorizontal size={16} />
			</DropdownMenu.Trigger>
			<DropdownMenu.Portal>
				<DropdownMenu.Content class="group-action-menu" align="start" sideOffset={6}>
					<DropdownMenu.Item
						class="group-action-item"
						disabled={!previousId || !firstId}
						onSelect={() => firstId && onmove(group.id, firstId, 'before')}
					>
						<ChevronsLeft size={16} /><span>Move first</span>
					</DropdownMenu.Item>
					<DropdownMenu.Item
						class="group-action-item"
						disabled={!previousId}
						onSelect={() => previousId && onmove(group.id, previousId, 'before')}
					>
						<ChevronLeft size={16} /><span>Move left</span>
					</DropdownMenu.Item>
					<DropdownMenu.Item
						class="group-action-item"
						disabled={!nextId}
						onSelect={() => nextId && onmove(group.id, nextId, 'after')}
					>
						<ChevronRight size={16} /><span>Move right</span>
					</DropdownMenu.Item>
					<DropdownMenu.Item
						class="group-action-item"
						disabled={!nextId || !lastId}
						onSelect={() => lastId && onmove(group.id, lastId, 'after')}
					>
						<ChevronsRight size={16} /><span>Move last</span>
					</DropdownMenu.Item>
					<DropdownMenu.Separator class="group-action-separator" />
					<DropdownMenu.Item class="group-action-item" onSelect={onedit}>
						{#if group.source_link}<RadioTower size={16} /><span>Sync settings</span>{:else}<Pencil
								size={16}
							/><span>Rename group</span>{/if}
					</DropdownMenu.Item>
					<DropdownMenu.Item class="group-action-item danger" onSelect={onremove}>
						<Trash2 size={16} /><span>Delete group</span>
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Portal>
		</DropdownMenu.Root>
	</div>
</div>

<style>
	.group-tab {
		position: relative;
		display: grid;
		flex: 0 0 auto;
		grid-template-columns: 1.8rem minmax(0, 1fr) 1.8rem;
		align-items: center;
		border: 1px solid transparent;
		border-radius: 0.65rem;
		background: transparent;
		transition:
			background var(--micro),
			border-color var(--micro),
			opacity var(--micro);
	}
	.group-tab.selected {
		border-color: color-mix(in oklch, var(--periwinkle) 65%, transparent);
		background: color-mix(in oklch, var(--periwinkle) 13%, transparent);
	}
	.group-tab.dragging {
		opacity: 0.45;
	}
	.group-tab.over-before::before,
	.group-tab.over-after::after {
		position: absolute;
		top: 0.2rem;
		bottom: 0.2rem;
		width: 3px;
		border-radius: 99px;
		background: var(--aqua);
		content: '';
	}
	.group-tab.over-before::before {
		left: -0.17rem;
	}
	.group-tab.over-after::after {
		right: -0.17rem;
	}
	.group-drag,
	.group-menu-slot :global(.group-menu-trigger) {
		display: grid;
		width: 1.8rem;
		height: 2.3rem;
		place-items: center;
		border: 0;
		border-radius: 0.5rem;
		background: transparent;
		padding: 0;
		color: var(--muted);
		visibility: hidden;
		cursor: pointer;
	}
	.group-drag {
		cursor: grab;
	}
	.group-tab:hover .group-drag,
	.group-tab:focus-within .group-drag,
	.group-tab:hover .group-menu-slot :global(.group-menu-trigger),
	.group-tab:focus-within .group-menu-slot :global(.group-menu-trigger),
	.group-tab.selected .group-menu-slot :global(.group-menu-trigger) {
		visibility: visible;
	}
	.group-drag:hover,
	.group-menu-slot :global(.group-menu-trigger:hover),
	.group-menu-slot :global(.group-menu-trigger[data-state='open']) {
		background: var(--surface-raised);
		color: var(--text);
	}
	.group-main {
		display: flex;
		min-height: 2.3rem;
		align-items: center;
		gap: 0.35rem;
		white-space: nowrap;
		border: 0;
		background: transparent;
		padding: 0.35rem 0.25rem;
		color: var(--muted);
		font-size: 0.68rem;
		font-weight: 750;
		cursor: pointer;
	}
	.group-main strong {
		border-radius: 99px;
		background: var(--surface-raised);
		padding: 0.1rem 0.35rem;
		font-size: inherit;
	}
	.group-main em {
		color: var(--aqua);
		font-size: 0.55rem;
		font-style: normal;
		text-transform: uppercase;
	}
	.group-main em.issue {
		color: var(--error);
	}
	:global(.group-action-menu) {
		z-index: 100;
		min-width: 12rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface-raised);
		padding: 0.35rem;
		box-shadow: 0 18px 48px rgb(0 0 0 / 0.28);
		color: var(--text);
	}
	:global(.group-action-item) {
		display: flex;
		min-height: 2.5rem;
		align-items: center;
		gap: 0.65rem;
		border-radius: 0.55rem;
		padding: 0.5rem 0.65rem;
		outline: none;
		font-size: 0.78rem;
		font-weight: 720;
		cursor: pointer;
	}
	:global(.group-action-item[data-highlighted]) {
		background: color-mix(in oklch, var(--periwinkle) 14%, var(--surface-raised));
	}
	:global(.group-action-item[data-disabled]) {
		opacity: 0.4;
		cursor: not-allowed;
	}
	:global(.group-action-item.danger) {
		color: var(--error);
	}
	:global(.group-action-separator) {
		height: 1px;
		margin: 0.3rem 0;
		background: var(--line);
	}
	@media (hover: none) {
		.group-drag,
		.group-menu-slot :global(.group-menu-trigger) {
			visibility: visible;
		}
	}
</style>
