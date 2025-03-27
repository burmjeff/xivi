<!-- TemplateChannel.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { templateGroups } from '@xivi/stores/template_store';
	import type { TemplateChannel } from '@xivi/data/template_entities';
	import type { ModalSettings } from '@skeletonlabs/skeleton-svelte';
	import {
		dndzone,
		TRIGGERS,
		SHADOW_ITEM_MARKER_PROPERTY_NAME,
		DRAGGED_ELEMENT_ID
	} from 'svelte-dnd-action';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicIn } from 'svelte/easing';

	interface Props {
		groupId: number;
		groupIdx: number;
	}

	let { groupId, groupIdx }: Props = $props();

	const modalStore = getModalStore();
	let dndTypeChannels = 'channels';
	let dndItem: TemplateChannel;
	let dndIdx: number;
	let shouldIgnoreDndEvents = false;
	const flipDurationMs = 150;

	const updateTemplateChannels = async () => {
		const response = await fetch(`/api/template/group/${groupId}/channels`);
		const data = await response.json();
		return data.templatechannels;
	};

	onMount(async () => {
		$templateGroups[groupIdx].channels = [];
		const fetchedData = await updateTemplateChannels();
		if (typeof fetchedData !== 'undefined') {
			fetchedData.forEach(function (channel: TemplateChannel) {
				$templateGroups[groupIdx].channels.push(channel);
				$templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels;
			});
		}
	});

	async function updateSettings(channelIdx: number, formData: any) {
		if (formData.name != '' && formData.tvgid != '' && formData.logo != '') {
			let newSettings = {
				id: $templateGroups[groupIdx].channels[channelIdx].id,
				name: formData.name,
				tvgid: formData.tvgid,
				logoid: formData.logoid,
				uuid: $templateGroups[groupIdx].channels[channelIdx].uuid,
				logo: formData.logo
			};
			window.console.log('TemplateChannel: ', newSettings);

			try {
				const response = await fetch('/api/template/channel', {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newSettings)
				});
				const data = await response.status;
				console.log('Updated template channel:', data);
				$templateGroups[groupIdx].channels[channelIdx].name = formData.name;
				$templateGroups[groupIdx].channels[channelIdx].tvgid = formData.tvgid;
				$templateGroups[groupIdx].channels[channelIdx].logoid = formData.logoid;
				$templateGroups[groupIdx].channels[channelIdx].logo = formData.logo;
			} catch (error) {
				console.log('Error updating template channel:', error);
				return;
			}
		}
	}

	function modalSettings(channelIdx: number) {
		new Promise<boolean>((resolve) => {
			const modal: ModalSettings = {
				type: 'component',
				component: 'modalChannelSettings',
				meta: {
					isNew: false,
					channelIdx: channelIdx,
					groupIdx: groupIdx
				},
				response: (r: boolean) => {
					resolve(r);
				}
			};
			modalStore.trigger(modal);
		}).then((r: any) => {
			if (r) {
				updateSettings(channelIdx, r);
			}
		});
	}

	async function convertChannel(channelId: string) {
		if (channelId !== '') {
			try {
				const response = await fetch(`/api/playlist/channel/${channelId}/convert/${groupId}`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					}
				});
				if (response.ok) {
					const data = await response.json();
					console.log('Created template channel:', data);
					$templateGroups[groupIdx].channels.push(data.templatechannel);
					$templateGroups[groupIdx].channels = [...$templateGroups[groupIdx].channels];
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error creating template channel:', error);
			}
		}
	}

	function handleDndConsider(e: CustomEvent<DndEvent<TemplateChannel>>) {
		const { trigger, id } = e.detail.info;
		//e.detail.items.sort((itemA, itemB) => Number(itemA.id) - Number(itemB.id));

		if (trigger === TRIGGERS.DRAG_STARTED) {
			dndIdx = $templateGroups[groupIdx].channels.findIndex((item) => item.id === Number(id));
			dndItem = $templateGroups[groupIdx].channels[dndIdx];
			$templateGroups[groupIdx].channels = e.detail.items;
			shouldIgnoreDndEvents = true;
		} else if (!shouldIgnoreDndEvents) {
			$templateGroups[groupIdx].channels = e.detail.items;
		} else {
			$templateGroups[groupIdx].channels = [...$templateGroups[groupIdx].channels];
		}
	}
	function handleDndFinalize(e: CustomEvent<DndEvent<TemplateChannel>>) {
		const { trigger, id } = e.detail.info;
		if (trigger === TRIGGERS.DROPPED_INTO_ZONE && !shouldIgnoreDndEvents) {
			e.detail.items = e.detail.items.filter((item) => !item.isDragged);
			$templateGroups[groupIdx].channels = e.detail.items;
			convertChannel(id);
			shouldIgnoreDndEvents = false;
		} else if (!shouldIgnoreDndEvents) {
			$templateGroups[groupIdx].channels = e.detail.items;
		} else if (trigger === TRIGGERS.DROPPED_INTO_ANOTHER) {
			e.detail.items = e.detail.items.filter((item) => !item[SHADOW_ITEM_MARKER_PROPERTY_NAME]);
			e.detail.items.splice(dndIdx, 0, dndItem);
			$templateGroups[groupIdx].channels = e.detail.items;
			shouldIgnoreDndEvents = false;
		} else {
			$templateGroups[groupIdx].channels = e.detail.items;
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
		}
	}
</script>

{#if $templateGroups[groupIdx] != null && $templateGroups[groupIdx].channels != null}
	<table class="templateChannel table ">
		<thead>
			<tr id="thead">
				<th>Logo</th>
				<th>Name</th>
				<th>tvg-id</th>
			</tr>
		</thead>
		<tbody
			use:dndzone={{
				items: $templateGroups[groupIdx].channels,
				flipDurationMs,
				type: dndTypeChannels,
				transformDraggedElement
			}}
			onconsider={handleDndConsider}
			onfinalize={handleDndFinalize}
		>
			{#if $templateGroups[groupIdx].channels.length > 0}
				{#each $templateGroups[groupIdx].channels as channel, channelIdx (channel.id)}
					<tr
						id="animate"
						animate:flip={{ duration: flipDurationMs }}
						onclick={() => modalSettings(channelIdx)}
					>
						<td><img class="max-w-16 max-h-10" src={channel.logo} alt="Logo" /></td>
						<td>{channel.name}</td>
						<td>{channel.tvgid}</td>

						{#if channel[SHADOW_ITEM_MARKER_PROPERTY_NAME]}
							{#if channel.name}
								<td in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
									{channel.name}
								</td>
							{:else}
								<td in:fade={{ duration: 200, easing: cubicIn }} class="custom-shadow-item">
									{channel.name}
								</td>
							{/if}
						{/if}
					</tr>
				{/each}
			{:else}
				<tr><td>No channels found</td></tr>
			{/if}
		</tbody>
	</table>
{/if}

<style>
	#animate {
		position: relative;
		text-align: center;
	}
	#thead {
		position: relative;
		text-align: center;
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
</style>
