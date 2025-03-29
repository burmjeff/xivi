<!-- TemplateGroup.svelte -->
<script lang="ts">
	import TemplateChannel from './TemplateChannel.svelte';
	import { onMount } from 'svelte';
	import {
		Accordion,
		Modal
	} from '@skeletonlabs/skeleton-svelte';
	import { templates } from '@xivi/stores/template_store';
	import { templateGroups } from '@xivi/stores/template_store';
	import type { TemplateGroup } from '@xivi/data/template_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';
	import Icon from '@iconify/svelte';

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
			if (await response.ok) {
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
			const data = await response.status;
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
		index: number | undefined
	) {
		if (!shouldIgnoreDndEvents) data!.isDragged = true;
	}
</script>

<Modal
	open={deleteModalOpen}
	onOpenChange={(e) => (deleteModalOpen = e.open)}
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
	<div class="card p-4 w-modal shadow-xl space-y-4">
		<header class="text-2xl font-bold">Please Confirm</header>
		<article>Are you sure you wish to remove this group from template?</article>
		<footer class="flex justify-end space-x-2">
			<button class="btn preset-outlined-surface-500" onclick={() => handleDeleteClose(false)}>Cancel</button>
			<button class="btn preset-tonal-error" onclick={() => handleDeleteClose(true)}>Delete</button>
		</footer>
	</div>
	{/snippet}
</Modal>

{#if $templates[templateIdx].groups != null}
	<Accordion>
		<section
			use:dndzone={{
				items: $templates[templateIdx].groups,
				flipDurationMs,
				type: dndTypeGroups,
				transformDraggedElement
			}}
			onconsider={handleDndConsider}
			onfinalize={handleDndFinalize}
		>
			{#if $templates[templateIdx].groups.length > 0}
				{#each $templates[templateIdx].groups as group, groupIdx (group.id)}
					<div id="animate" class="card shadow-md mb-1" animate:flip={{ duration: flipDurationMs }}>
						<Accordion.Item  value={group.name}>
							{#snippet control()}

									<div class="flex flex-row items-center">
										<h4 class="text-lg">{group.name}</h4>
										<button
											class="btn-icon btn-icon-sm inset-y-0 bg-transparent!"
											onclick={() => {
												(group.itemOpen = true), deletePrompt(group.id);
											}}
										>
											<Icon icon="icon-park-outline:delete" width="18" height="18" />
										</button>
									</div>
							{/snippet}
							{#snippet panel()}
								<TemplateChannel groupId={group.id} {groupIdx} />
							{/snippet}
						</Accordion.Item>
						{#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
							<div in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
								{group.name}
							</div>
						{/if}
					</div>
				{/each}
			{:else}
				<p>No groups found</p>
			{/if}
		</section>
	</Accordion>
{/if}

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
	}
	#animate {
		position: relative;
		text-align: center;
	}
</style>
