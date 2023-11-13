<!-- TemplateGroup.svelte -->
<script lang="ts">
    import TemplateChannel from './TemplateChannel.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, getModalStore, type PopupSettings, type ModalSettings } from '@skeletonlabs/skeleton';
    import {templateGroups} from '@xivi/stores/template_store';
    import type { TemplateGroup } from '@xivi/data/template_entities';
    import { dndzone, TRIGGERS, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action';
    import {flip} from 'svelte/animate';
    import {fade} from 'svelte/transition';
    import {cubicIn} from 'svelte/easing';
    import IconParkOutlineEditTwo from '~icons/icon-park-outline/edit-two'
    import IconParkOutlineDelete from '~icons/icon-park-outline/delete';
    import IconParkOutlineAdd from '~icons/icon-park-outline/add';

    let dndTypePlaylist = "playlist";
    let dndTypeTemplateGroup = "templateGroup";
    let shouldIgnoreDndEvents = false;
    const flipDurationMs = 150;
    let dndItem: TemplateGroup;
	let dndIdx: number
    const modalStore = getModalStore();

    const updateTemplateGroups = async () => {
        const response = await fetch('/api/template/groups/all');
        const data = await response.json();
        return data.templategroups;
    }

    onMount(async () => {
        templateGroups.set(await updateTemplateGroups());
    });

    let templateGroupSettings: PopupSettings = {
        // Set the event as: click | hover | hover-click
        event: 'click',
        // Provide a matching 'data-popup' value.
        target: 'addTemplateGroupPopup'
    };

    async function addTemplateGroup() {
        const inputName = {name: (document.querySelector('.template_group_name input') as HTMLInputElement).value};
        if (inputName !== null) {
            try {
                const response = await fetch('/api/template/group', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(inputName)
                });
                const data = await response.json();
                console.log('Created template group', data);
                $templateGroups.push(data.templategroup);
                $templateGroups = $templateGroups;
            } catch (error) {
                console.log('Error creating templateGroup:', error);
            }
        }
    }

    function renamePrompt(groupIdx: number, groupName: string, groupId: string): void {
		const prompt: ModalSettings = {
			type: 'prompt',
			title: 'Rename Group',
			body: 'Enter new template group name in field below.',
			value: groupName,
			valueAttr: { type: 'text', minlength: 1, maxlength: 20, required: true },
			response: (newName: string) => {
				if (newName) renameGroup(groupIdx, newName, groupId);
			},
            buttonTextCancel: 'Cancel',
		    buttonTextSubmit: 'Submit',
		};
		modalStore.trigger(prompt);
	}

    async function renameGroup(groupIdx: number, groupName: string, groupId: string) {
        if (groupName !=='') {
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
                    $templateGroups[groupIdx].name = groupName
                } else {
                    console.error('Error:', response.status, response.statusText);
                }
            } catch (error) {
                console.log('Error updating template group:', error);
            }
        }
    }

    function deletePrompt(groupId: string): void {
        $templateGroups = [...$templateGroups]
		const modal: ModalSettings = {
			type: 'confirm',
            title: 'Please Confirm',
            body: 'Are you sure you wish to delete this group?',
            // TRUE if confirm pressed, FALSE if cancel pressed
            response: (r: boolean) => {
				if (r) deleteGroup(groupId);
			},
		};
		modalStore.trigger(modal);
	}
    
    //TODO COLLAPSE ACCORDIION ITEM BEFORE DELETE
    async function deleteGroup(groupId: string) {
		try {
			const response = await fetch(`/api/template/group/${groupId}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted template group:', data);
			$templateGroups = $templateGroups.filter(t => t.id != groupId)
			modalStore.close();
		} catch (error) {
			console.log('Error deleting template group:', error);
			return;
		}
	}

    async function addChannel(formData: any, groupId: string, groupIdx: number) {
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
				console.log('Added template channel:', data);
				$templateGroups[groupIdx].channels.push(data.templatechannel)
				$templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels
			} catch (error) {
				console.log('Error updating template channel:', error);
				return;
			}
		}
	}

	function modalAdd(groupId: string, groupIdx: number) {
		new Promise<boolean>((resolve) => {
			const modal: ModalSettings = {
				type: 'component',
				component: 'modalChannelSettings',
				meta: { 
					isNew: true,
					channelIdx: null,
					groupIdx: groupIdx
				 },
				response: (r: boolean) => {
					resolve(r);
				}
			};
			modalStore.trigger(modal);
		}).then((r: any) => {
			if (r) {addChannel(r, groupId, groupIdx)};
		});
	}

    function convertPrompt(groupId: string): void {
		const prompt: ModalSettings = {
			type: 'prompt',
			title: 'Convert Playlist Group to Template Group',
			body: 'Enter new template group name in field below.',
			value: 'Example Template',
			valueAttr: { type: 'text', minlength: 1, maxlength: 20, required: true },
			response: (groupName: string) => {
				if (groupName) convertGroup(groupName, groupId);
			},
			buttonTextCancel: 'Cancel',
			buttonTextSubmit: 'Submit'
		};
		modalStore.trigger(prompt);
	}

	async function convertGroup(groupName: string, groupId: string) {
		if (groupName !== '') {
			const newGroup = {
				name: groupName
			};
			try {
				const response = await fetch(`/api/playlist/group/${groupId}/convert`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newGroup)
				});
				const data = await response.json();
				console.log('Created template group:', data);
				$templateGroups.push(data.templategroup)
                $templateGroups = [...$templateGroups]
			} catch (error) {
				console.log('Error creating template group:', error);
			}
		}
	}

    function handleDndConsider(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const {trigger, id} = e.detail.info;
		e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $templateGroups.findIndex(item => item.id === id);
			dndItem =  $templateGroups[dndIdx];
            e.detail.items[dndIdx].itemOpen = false;
            $templateGroups[dndIdx].itemOpen = false;
			$templateGroups = e.detail.items
			shouldIgnoreDndEvents = true;
		}
        else if (!shouldIgnoreDndEvents) {
            $templateGroups = e.detail.items;
        }
        else {
            $templateGroups = [...$templateGroups]
        }
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<TemplateGroup>>) {
		const {trigger, id} = e.detail.info;
        if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreDndEvents) {
            e.detail.items = e.detail.items.filter(item => !item.isDragged);
            $templateGroups = e.detail.items
            convertPrompt(id)
            shouldIgnoreDndEvents = false;
        }
        else if (!shouldIgnoreDndEvents) {
            $templateGroups = e.detail.items
        }
        else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER){
			e.detail.items = e.detail.items.filter(item => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx,0, dndItem)
            $templateGroups = e.detail.items
            shouldIgnoreDndEvents = false;
        } else {
            $templateGroups = e.detail.items
            shouldIgnoreDndEvents = false;
        }
    }
    function transformDraggedElement(draggedEl: HTMLElement | undefined, data: Item | undefined, index: number | undefined) {
        if (!shouldIgnoreDndEvents) data!.isDragged = true
	}
</script>

<section class="tmplgroups card card-hover p-1" >
    <header class="tmplgroups-header flex justify-center items-center space-x-4">
        <h3 class="h3 font-bold">Groups</h3>
        <button class="btn btn-sm variant-ringed-primary" use:popup={templateGroupSettings}>+ add new</button>
    </header>
    {#if $templateGroups != null}
        <Accordion>
            <section id="accord" class="templategroups-viewport min-w-full overflow-auto" 
            use:dndzone={{items: $templateGroups, flipDurationMs, type: dndTypePlaylist, transformDraggedElement}} 
            use:dndzone={{items: $templateGroups, flipDurationMs, type: dndTypeTemplateGroup, transformDraggedElement}} 
            on:consider={handleDndConsider} on:finalize={handleDndFinalize}>
                {#if $templateGroups.length > 0}
                    {#each $templateGroups as group, groupIdx (group.id)}
                        <div id="animate" animate:flip={{duration: flipDurationMs}}>
                            <AccordionItem class="card mb-1" key={groupIdx} bind:open={group.itemOpen}>
                                <svelte:fragment slot="summary">
                                    <div class="flex flex-row">
                                        <h4>{group.name}</h4>
                                        <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" 
                                            on:click={() => {group.itemOpen = true, renamePrompt(groupIdx, group.name, group.id)}}>
                                            <i><IconParkOutlineEditTwo/></i>
                                        </button>
                                        <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" 
                                            on:click={() => {group.itemOpen = true, deletePrompt(group.id)}}>
                                            <i><IconParkOutlineDelete/></i>
                                        </button>
                                        <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" 
                                            on:click={() => modalAdd(group.id, groupIdx)}>
                                            <i><IconParkOutlineAdd/></i>
                                        </button>
                                    </div>
                                </svelte:fragment>
                                <svelte:fragment slot="content">
                                    <TemplateChannel groupId={group.id} groupIdx={groupIdx}/>
                                </svelte:fragment>
                            </AccordionItem>

                            {#if group[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
                                <div in:fade={{duration:200, easing: cubicIn}} class='custom-shadow-item'>{group.name}</div>
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

<div class="card p-4 gap-4" data-popup="addTemplateGroupPopup">
	<h2>Add Template Group</h2>
    <div class="space-y-4">
        <label class="template_group_name">
            <span>Template Group Name</span>
            <input class="input" type="text" placeholder="Template Group Name" />
        </label>
        <label class="submit_button">
            <button class="btn bg-primary-500" on:click={addTemplateGroup}>Add Template Group</button>
        </label>
    </div>
</div>

<style>
    #accord {
        max-height: 82vh;
        height: 82vh;
    }
    .custom-shadow-item {
		position: absolute;
		top: 0; left:0; right: 0; bottom: 0;
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