<!-- TemplateGroupList.svelte -->
<script lang="ts">
	import TemplateChannel from './TemplateChannel.svelte';
	import { onMount } from 'svelte';
	import {
		Accordion,
		Modal
	} from '@skeletonlabs/skeleton-svelte';
	import { templateGroups } from '@xivi/stores/template_store';
	import type { TemplateGroup } from '@xivi/data/template_entities';
	import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
	import { cubicIn } from 'svelte/easing';
	import Icon from '@iconify/svelte';
	import GroupSettings from './modals/GroupSettings.svelte';
	import {
		FloatingArrow,
		arrow,
		autoUpdate,
		flip,
		offset,
		useDismiss,
		useFloating,
		useHover,
		useInteractions,
		useRole,
	} from "@skeletonlabs/floating-ui-svelte";
	import { flip as flipAnimation } from 'svelte/animate';
	import { fade } from "svelte/transition";
	import ChannelSettings from './modals/ChannelSettings.svelte';

	let dndPlaylistId: number;
	let dndTypeGroups = 'groups';
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;
	let dndItem: TemplateGroup;
	let dndIdx: number;

	let tooltipAdd = $state(false);
	let elemArrow: HTMLElement | null = $state(null);

	const updateTemplateGroups = async () => {
		const response = await fetch('/api/template/groups/all');
		const data = await response.json();
		return data.templategroups;
	};

	onMount(async () => {
		templateGroups.set(await updateTemplateGroups());
	});

	// Floating UI setup
	const tooltipFloatingAdd = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return tooltipAdd;
		},
		onOpenChange: (v) => {
			tooltipAdd = v;
		},
		placement: "top",
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		},
	});
	const tooltipRole = useRole(tooltipFloatingAdd.context, { role: "tooltip" });
	const tooltipHover = useHover(tooltipFloatingAdd.context, { move: false });
	const tooltipDismiss = useDismiss(tooltipFloatingAdd.context);
	const tooltipInteractions = useInteractions([tooltipRole, tooltipHover, tooltipDismiss]);

	async function renameGroup(groupIdx: number, groupName: string, groupId: string) {
		if (groupName !== '') {
			const newGroup = {
				id: groupId,
				name: groupName
			};
			try {
				const response = await fetch(`/api/template/group`, {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newGroup)
				});
				if (response.ok) {
					$templateGroups[groupIdx].name = groupName;
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error updating template group:', error);
			}
		}
	}

	let modalDeleteOpen = $state(false);
	let modalChannelOpen = $state(false);
	let modalConvertOpen = $state(false);
	let groupToDelete = $state<number>(0);
	let groupToAddChannel = $state({ id: 0, idx: 0 });
	let convertGroupData = $state({ id: 0, name: '' });
	let convertGroupNameInput = $state('');

	function deletePrompt(groupId: number): void {
		groupToDelete = groupId;
		modalDeleteOpen = true;
	}

	function handleDeleteClose(confirmed: boolean) {
		if (confirmed) {
			deleteGroup(groupToDelete);
		}
		modalDeleteOpen = false;
	}

	//TODO COLLAPSE ACCORDIION ITEM BEFORE DELETE
	async function deleteGroup(groupId: number) {
		try {
			const response = await fetch(`/api/template/group/${groupId}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted template group:', data);
			$templateGroups = $templateGroups.filter((t) => t.id != groupId);
		} catch (error) {
			console.log('Error deleting template group:', error);
			return;
		}
	}

	async function addChannel(formData: any, groupId: number, groupIdx: number) {
		if (formData.name != '' && formData.tvgid != '' && formData.logo != '') {
			let newChannel = {
				name: formData.name,
				tvgid: formData.tvgid,
				logoid: formData.logoid
			};

			try {
				const response = await fetch(`/api/template/group/${groupId}/channel`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newChannel)
				});
				const data = await response.json();
				if (response.ok) {
					console.log('Added template channel:', data);
					$templateGroups[groupIdx].channels.push(data.templatechannel);
					$templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels;
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error updating template channel:', error);
				return;
			}
		}
	}

	function modalAddChannel(groupId: number, groupIdx: number) {
		groupToAddChannel = { id: groupId, idx: groupIdx };
		modalChannelOpen = true;
	}

	function handleChannelClose(formData: any) {
		if (formData) {
			addChannel(formData, groupToAddChannel.id, groupToAddChannel.idx);
		}
		modalChannelOpen = false;
	}

	function convertPrompt(groupId: number, name: string): void {
		convertGroupData = { id: groupId, name: name };
		convertGroupNameInput = name;
		modalConvertOpen = true;
	}

	function handleConvertClose(confirmed: boolean) {
		if (confirmed && convertGroupNameInput) {
			convertGroup(convertGroupNameInput, convertGroupData.id);
		}
		modalConvertOpen = false;
	}

	async function addTemplateGroup(
		formData: any,
		groupIdx: number,
		isNew: boolean,
		group: TemplateGroup | undefined
	) {
		let method: string;
		if (formData.name) {
			let newGroup = {
				id: group?.id,
				name: formData.name,
				dynamic: formData.dynamic,
				dynamicgroup: formData.dynamicgroup
			};

			if (isNew) {
				method = 'POST';
			} else {
				method = 'PUT';
			}

			try {
				const response = await fetch('/api/template/group', {
					method: method,
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newGroup)
				});
				if (response.ok) {
					if (isNew) {
						const data = await response.json();
						console.log('Created template group: ', data);
						$templateGroups.push(data.templategroup);
						$templateGroups = $templateGroups;
					} else {
						console.log('Updated template group: ', formData.name);
						$templateGroups[groupIdx].name = formData.name;
						$templateGroups[groupIdx].dynamic = formData.dynamic;
						$templateGroups[groupIdx].dynamicgroup = formData.dynamicgroup;
					}
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error creating templateGroup:', error);
			}
		}
	}

	let modalOpen = $state(false);
	let currentGroupData = $state({
		isNew: false,
		groupIdx: 0,
		group: undefined as TemplateGroup | undefined
	});

	function modalGroupSettings(isNew: boolean, groupIdx: number, group: TemplateGroup | undefined) {
		currentGroupData = {
			isNew,
			groupIdx,
			group
		};
		modalOpen = true;
	}
	function handleGroupSettingsClose(formData?: any) {
		if (formData) {
			addTemplateGroup(formData, currentGroupData.groupIdx, currentGroupData.isNew, currentGroupData.group);
		}
		modalOpen = false;
	}

	async function convertGroup(groupName: string, groupId: number) {
		if (groupName !== '' && dndPlaylistId !== 0) {
			const newGroup = {
				name: groupName
			};
			try {
				const response = await fetch(`/api/playlist/${dndPlaylistId}/group/${groupId}/convert`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newGroup)
				});
				if (response.ok) {
					const data = await response.json();
					console.log('Created template group:', data);
					$templateGroups.push(data.templategroup);
					$templateGroups = [...$templateGroups];
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error creating template group:', error);
			}
		}
	}

	function handleDndConsider(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const { trigger, id } = e.detail.info;
		e.detail.items.sort((itemA, itemB) => itemA.id - itemB.id);

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $templateGroups.findIndex((item) => item.id === Number(id));
			dndItem = $templateGroups[dndIdx];
			e.detail.items[dndIdx].itemOpen = false;
			$templateGroups[dndIdx].itemOpen = false;
			$templateGroups = e.detail.items;
			shouldIgnoreDndEvents = true;
		} else if (!shouldIgnoreDndEvents) {
			$templateGroups = e.detail.items;
		} else {
			$templateGroups = [...$templateGroups];
		}
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const { trigger, id } = e.detail.info;
		if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreDndEvents) {
			let name = e.detail.items.filter((item) => item.isDragged)[0].name;
			e.detail.items = e.detail.items.filter((item) => !item.isDragged);
			$templateGroups = e.detail.items;
			convertPrompt(Number(id), name);
			shouldIgnoreDndEvents = false;
		} else if (!shouldIgnoreDndEvents) {
			$templateGroups = e.detail.items;
		} else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER) {
			e.detail.items = e.detail.items.filter((item) => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx, 0, dndItem);
			$templateGroups = e.detail.items;
			shouldIgnoreDndEvents = false;
		} else {
			$templateGroups = e.detail.items;
			shouldIgnoreDndEvents = false;
		}
	}
	function transformDraggedElement(
		draggedEl: HTMLElement | undefined,
		data: Item | undefined,
		index: number | undefined
	) {
		if (!shouldIgnoreDndEvents) {
			data!.isDragged = true;
			dndPlaylistId = data!.playlist_id;
		}
	}
</script>

<section class="tmplgroups card card-hover p-1">
	<header class="tmplgroups-header flex items-center justify-center">
		<h3 class="h3 font-bold">Template Groups</h3>
		<button
			class="btn btn-md"
			onclick={() => modalGroupSettings(true, 0, undefined)}
			bind:this={tooltipFloatingAdd.elements.reference}
			{...tooltipInteractions.getReferenceProps()}
		>
			<Icon icon="icon-park-twotone:add-one" color="#0a7e85" width="25" height="25" />
		</button>
		{#if tooltipAdd}
					<div
						bind:this={tooltipFloatingAdd.elements.floating}
						style={tooltipFloatingAdd.floatingStyles}
						{...tooltipInteractions.getFloatingProps()}
						class="floating popover-neutral"
						transition:fade={{ duration: 200 }}
					>
						<p>
							<strong>Add Group</strong>
						</p>
						<FloatingArrow bind:ref={elemArrow} context={tooltipFloatingAdd.context} fill="#575969" />
					</div>
				{/if}
	</header>
	{#if $templateGroups != null}
		<Accordion collapsible>
			<section
				id="accord"
				class="templategroups-viewport min-w-full overflow-auto"
				use:dndzone={{
					items: $templateGroups,
					flipDurationMs,
					type: dndTypeGroups,
					transformDraggedElement
				}}
				onconsider={handleDndConsider}
				onfinalize={handleDndFinalize}
			>
				{#if $templateGroups.length > 0}
					{#each $templateGroups as group, groupIdx (group.id)}
						<div id="animate" class="card shadow-md mb-1" animate:flipAnimation={{ duration: flipDurationMs }}>
							<Accordion.Item value={group.name}>
								{#snippet control()}					
									<div class="flex flex-row items-center">
										<h4 class="text-lg">{group.name}</h4>
										<button
											class="btn-icon btn-icon-sm inset-y-0 bg-transparent! ml-auto"
											onclick={() => {
											(group.itemOpen = true), modalGroupSettings(false, groupIdx, group);
										}}
										>
											<Icon icon="icon-park-outline:edit-two" width="18" height="18" />
										</button>
										<button
											class="btn-icon btn-icon-sm inset-y-0 bg-transparent!"
											onclick={() => {
											(group.itemOpen = true), deletePrompt(group.id);
										}}
										>
											<Icon icon="icon-park-outline:delete" width="18" height="18" />
										</button>
										<button
											class="btn-icon btn-icon-sm inset-y-0 bg-transparent!"
											onclick={() => modalAddChannel(group.id, groupIdx)}
										>
											<Icon icon="icon-park-outline:add" width="18" height="18" />
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
</section>

<div class="card preset-filled-secondary-500 p-2" data-popup="addTooltip">
	<p>Add New Template Group</p>
	<div class="preset-filled-secondary-500 arrow"></div>
</div>

<GroupSettings
    bind:modalOpen={modalOpen}
    parent={{ onClose: handleGroupSettingsClose }}
    isNew={currentGroupData.isNew}
    name={currentGroupData.group?.name ?? ''}
    dynamic={currentGroupData.group?.dynamic ?? false}
    dynamicgroup={currentGroupData.group?.dynamicgroup ?? 0}
/>

<Modal
    open={modalDeleteOpen}
    onOpenChange={(e) => (modalDeleteOpen = e.open)}
    backdropClasses="backdrop-blur-sm"
>
    {#snippet content()}
        <div class="card p-4 w-modal shadow-xl space-y-4">
            <header class="text-2xl font-bold">Please Confirm</header>
            <article>Are you sure you wish to delete this group?</article>
            <footer class="flex justify-end space-x-2">
                <button class="btn variant-filled" onclick={() => handleDeleteClose(false)}>Cancel</button>
                <button class="btn variant-filled-error" onclick={() => handleDeleteClose(true)}>Delete</button>
            </footer>
        </div>
    {/snippet}
</Modal>

<Modal
    open={modalChannelOpen}
    onOpenChange={(e) => (modalChannelOpen = e.open)}
    backdropClasses="backdrop-blur-sm"
>
    {#snippet content()}
        <ChannelSettings
            isNew={true}
            channelIdx={null}
            groupIdx={groupToAddChannel.idx}
            parent={{ onClose: handleChannelClose }}
        />
    {/snippet}
</Modal>

<Modal
    open={modalConvertOpen}
    onOpenChange={(e) => (modalConvertOpen = e.open)}
    backdropClasses="backdrop-blur-sm"
>
    {#snippet content()}
        <div class="card p-4 w-modal shadow-xl space-y-4">
            <header class="text-2xl font-bold">Convert Playlist Group to Template Group</header>
            <article>
                <label class="label">
                    <span>Enter new template group name</span>
                    <input
                        class="input"
                        type="text"
                        bind:value={convertGroupNameInput}
                        minlength="1"
                        maxlength="20"
                        required
                    />
                </label>
            </article>
            <footer class="flex justify-end space-x-2">
                <button class="btn variant-filled" onclick={() => handleConvertClose(false)}>Cancel</button>
                <button class="btn variant-filled-primary" onclick={() => handleConvertClose(true)}>Submit</button>
            </footer>
        </div>
    {/snippet}
</Modal>

<style>
	#accord {
		max-height: 76vh;
		height: 76vh;
	}
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
