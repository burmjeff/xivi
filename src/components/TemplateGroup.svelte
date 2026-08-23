<!-- TemplateGroup.svelte -->
<script lang="ts">
	import TemplateChannel from './TemplateChannel.svelte';
	import { onMount } from 'svelte';
	import { Accordion } from '@skeletonlabs/skeleton-svelte';
	import Modal from '@xivi/components/Modal.svelte';
	import { templates } from '@xivi/stores/template_store';
	import { templateGroups } from '@xivi/stores/template_store';
	import type { TemplateGroup } from '@xivi/data/template_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';
	import Icon from '@iconify/svelte';
	import {
		autoUpdate,
		flip as floatingFlip,
		offset,
		useDismiss,
		useFloating,
		useHover,
		useInteractions,
		useRole
	} from '@skeletonlabs/floating-ui-svelte';

	interface Props {
		templateId: number;
		templateIdx: number;
	}

	let { templateId, templateIdx }: Props = $props();
	let deleteModalOpen = $state(false);
	let currentGroupId = $state(0);
	let dndTypeGroups = 'groups';
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;
	let dndItem: TemplateGroup;
	let dndIdx: number;
	let accordionItem = $state<string[]>([]);
	let isDragFromHandle = $state(false);

	// Floating UI state
	let deleteTooltipOpen = $state<{ [key: number]: boolean }>({});

	const updateTemplateGroups = async () => {
		const response = await fetch(`/api/template/${templateId}/groups`);
		const data = await response.json();
		return data.templategroups;
	};

	onMount(async () => {
		$templates[templateIdx].groups = [];
		const fetchedData = await updateTemplateGroups();
		if (typeof fetchedData !== 'undefined') {
			$templates[templateIdx].groups = fetchedData;
			$templates[templateIdx].groups = $templates[templateIdx].groups;
		}
	});

	async function addGroup(groupId: number) {
		try {
			const response = await fetch(`/api/template/${templateId}/group/${groupId}/item`, {
				method: 'POST'
			});
			if (response.ok) {
				let newGroup = $templateGroups.find((item) => item.id === groupId);
				console.log('Added template group:', newGroup);
				const fetchedData = await updateTemplateGroups();
				if (typeof fetchedData !== 'undefined') {
					$templates[templateIdx].groups = fetchedData;
					$templates[templateIdx].groups = $templates[templateIdx].groups;
				}
			} else console.log('Error adding template group:', response);
		} catch (error) {
			console.log('Error adding template group:', error);
		}
	}

	function deletePrompt(groupId: number) {
		currentGroupId = groupId;
		deleteModalOpen = true;
	}

	function handleDeleteClose(confirm: boolean) {
		if (confirm) {
			deleteTemplateGroup(currentGroupId);
		}
		deleteModalOpen = false;
	}

	//TODO COLLAPSE ACCORDIION ITEM BEFORE DELETE
	async function deleteTemplateGroup(groupId: number) {
		try {
			const response = await fetch(`/api/template/${templateId}/group/${groupId}/item`, {
				method: 'DELETE'
			});
			const data = response.status;
			console.log('Removing template group:', data);
			$templates[templateIdx].groups = $templates[templateIdx].groups.filter(
				(t) => t.id != groupId
			);
		} catch (error) {
			console.log('Error removing template group:', error);
			return;
		}
	}

	function handleDndConsider(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const { trigger, id } = e.detail.info;
		//e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $templates[templateIdx].groups.findIndex((item) => item.id === Number(id));
			dndItem = $templates[templateIdx].groups[dndIdx];
			e.detail.items[dndIdx].itemOpen = false;
			$templates[templateIdx].groups[dndIdx].itemOpen = false;
			$templates[templateIdx].groups = e.detail.items;
			shouldIgnoreDndEvents = true;
		} else if (!shouldIgnoreDndEvents) {
			$templates[templateIdx].groups = e.detail.items;
		} else {
			$templates[templateIdx].groups = [...$templates[templateIdx].groups];
		}
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const { trigger, id } = e.detail.info;
		if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreDndEvents) {
			e.detail.items = e.detail.items.filter((item) => !item.isDragged);
			addGroup(Number(id));
			$templates[templateIdx].groups = e.detail.items;
			shouldIgnoreDndEvents = false;
		} else if (!shouldIgnoreDndEvents) {
			$templates[templateIdx].groups = e.detail.items;
		} else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER) {
			e.detail.items = e.detail.items.filter((item) => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx, 0, dndItem);
			$templates[templateIdx].groups = e.detail.items;
			shouldIgnoreDndEvents = false;
		} else {
			$templates[templateIdx].groups = e.detail.items;
			shouldIgnoreDndEvents = false;
		}
	}
	function transformDraggedElement(
		draggedEl: HTMLElement | undefined,
		data: Item | undefined,
		_index: number | undefined
	) {
		if (!shouldIgnoreDndEvents) data!.isDragged = true;
	}

	// Helper function to create floating UI for template group buttons
	function createTooltipFloating(groupId: number, tooltipType: 'delete') {
		const tooltipState = deleteTooltipOpen;

		return useFloating({
			whileElementsMounted: autoUpdate,
			get open() {
				return tooltipState[groupId] || false;
			},
			onOpenChange: (v) => {
				tooltipState[groupId] = v;
			},
			placement: 'top',
			strategy: 'fixed',
			get middleware() {
				return [offset(10), floatingFlip()];
			}
		});
	}

	// Helper function to create interactions for template group tooltips
	function createTooltipInteractions(floating: any) {
		const role = useRole(floating.context, { role: 'tooltip' });
		const hover = useHover(floating.context, { move: false });
		const dismiss = useDismiss(floating.context);
		return useInteractions([role, hover, dismiss]);
	}

	// Reset drag state on global mouse up to handle edge cases
	function handleGlobalMouseUp() {
		isDragFromHandle = false;
	}

	// Add global event listener
	if (typeof window !== 'undefined') {
		window.addEventListener('mouseup', handleGlobalMouseUp);
		window.addEventListener('pointerup', handleGlobalMouseUp);
	}
</script>

<div id="accord" class="templategroups-viewport min-w-full overflow-auto">
	{#if $templates[templateIdx].groups != null}
		<Accordion value={accordionItem} onValueChange={(e) => (accordionItem = e.value)} collapsible>
			<section
				use:dndzone={{
					items: $templates[templateIdx].groups,
					flipDurationMs,
					type: dndTypeGroups,
					transformDraggedElement,
					dragDisabled: !isDragFromHandle
				}}
				onconsider={handleDndConsider}
				onfinalize={handleDndFinalize}
			>
				{#if $templates[templateIdx].groups.length > 0}
					{#each $templates[templateIdx].groups as group, groupIdx (group.id)}
						{@const deleteFloating = createTooltipFloating(group.id, 'delete')}
						{@const deleteInteractions = createTooltipInteractions(deleteFloating)}
						<div
							id="animate"
							class="card mb-1 shadow-md"
							animate:flip={{ duration: flipDurationMs }}
						>
							<Accordion.Item value={group.name}>
								<Accordion.ItemTrigger class="flex w-full cursor-pointer flex-row items-center p-4">
									<button
										class="drag-handle hover:bg-surface-700/30 mr-2 flex-shrink-0 cursor-grab rounded px-2 py-1"
										onclick={(e) => e.stopPropagation()}
										onpointerdown={(e) => {
											e.stopPropagation();
											isDragFromHandle = true;
											// Reset after a delay to allow drag to initiate
											setTimeout(() => {
												isDragFromHandle = false;
											}, 100);
										}}
									>
										<Icon
											icon="material-symbols:drag-indicator"
											width="16"
											height="16"
											class="text-surface-400"
										/>
									</button>
									<h4 class="flex-grow text-left text-lg">{group.name}</h4>
									<div class="flex flex-row gap-1">
										<button
											class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
											onclick={(e) => {
												e.stopPropagation();
												deletePrompt(group.id);
											}}
											bind:this={deleteFloating.elements.reference}
											{...deleteInteractions.getReferenceProps()}
										>
											<Icon icon="icon-park-outline:delete" width="18" height="18" />
											{#if deleteTooltipOpen[group.id]}
												<div
													bind:this={deleteFloating.elements.floating}
													style={deleteFloating.floatingStyles}
													{...deleteInteractions.getFloatingProps()}
													class="floating glass card pointer-events-none z-[9999] p-2 shadow-lg"
													transition:fade={{ duration: 200 }}
												>
													<p class="text-sm font-medium"><strong>Remove Group</strong></p>
												</div>
											{/if}
										</button>
									</div>
								</Accordion.ItemTrigger>
								<Accordion.ItemContent>
									<TemplateChannel groupId={group.id} {groupIdx} />
								</Accordion.ItemContent>
							</Accordion.Item>
							{#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
								<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
									{group.name}
								</div>
							{/if}
						</div>
					{/each}
				{:else}
					<div class="flex flex-col items-center justify-center px-2 py-2 text-center">
						<Icon icon="mdi:folder-off" class="text-surface-500 mb-3" width="36" height="36" />
						<h4 class="text-surface-300 mb-2 text-lg font-medium">No Groups Found</h4>
						<p class="text-surface-400 max-w-md text-sm">
							Add groups to this template to organize your channels.
						</p>
					</div>
				{/if}
			</section>
		</Accordion>
	{/if}
</div>

<Modal
	open={deleteModalOpen}
	onOpenChange={(e) => (deleteModalOpen = e.open)}
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl max-w-screen-sm"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
		<header class="text-2xl font-bold">Please Confirm</header>
		<article>Are you sure you wish to remove this group from template?</article>
		<footer class="flex justify-end space-x-2">
			<button class="btn preset-outlined-surface-500" onclick={() => handleDeleteClose(false)}
				>Cancel</button
			>
			<button class="btn preset-tonal-error" onclick={() => handleDeleteClose(true)}>Delete</button>
		</footer>
	{/snippet}
</Modal>

<style>
	.custom-shadow-item {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		visibility: visible;
		border: 3px dashed grey;
		background: lightblue;
		opacity: 0.6;
		margin: 0;
		pointer-events: none;
	}
	#animate {
		position: relative;
		text-align: center;
	}

	/* High z-index for floating tooltips to ensure they appear above everything */
	:global(.floating) {
		z-index: 9999 !important;
		position: fixed !important;
	}

	/* Ensure accordion items don't clip tooltips */
	:global(.accordion-item) {
		overflow: visible !important;
	}

	/* Ensure card containers don't clip tooltips */
	.card {
		overflow: visible !important;
	}
</style>
