<!-- TemplateChannel.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { templateGroups } from '@xivi/stores/template_store';
	import type { TemplateChannel } from '@xivi/data/template_entities';
	import type { ModalSettings } from '@skeletonlabs/skeleton';
	import { getModalStore } from '@skeletonlabs/skeleton';

	export let groupId: number;
	export let groupIdx: number;
	const modalStore = getModalStore();

	const updateTemplateChannels = async () => {
		const response = await fetch(`/api/template/group/${groupId}/channels`);
		const data = await response.json();
		return data.templatechannels;
	};

	onMount(async () => {
		$templateGroups[groupIdx].channels = [];
		const fetchedData = await updateTemplateChannels();
        fetchedData.forEach(function (channel: TemplateChannel) {
			$templateGroups[groupIdx].channels.push(channel);
            $templateGroups[groupIdx].channels = $templateGroups[groupIdx].channels
		});
	});

	async function updateSettings(channelIdx: number, formData: any) {
		if (formData.name != '' && formData.tvgid != '' && formData.logo != '') {
			let newSettings: TemplateChannel;
			newSettings = {
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
			if (r) {updateSettings(channelIdx, r)};
		});
	}
</script>

{#if $templateGroups[groupIdx].channels != null}
	<div class="table-container">
		<table class="table table-hover text-center justify-center items-center h-full w-full">
			<thead>
				<tr>
					<th>Logo</th>
					<th>Name</th>
					<th>tvg-id</th>
				</tr>
			</thead>
			<tbody>
				{#each $templateGroups[groupIdx].channels as channel, channelIdx}
					<tr on:click={() => modalSettings(channelIdx)}>
						<td><img class="w-14" src={channel.logo} alt="Logo" /></td>
						<td>{channel.name}</td>
						<td>{channel.tvgid}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
    <p>No template channels found</p>
{/if}