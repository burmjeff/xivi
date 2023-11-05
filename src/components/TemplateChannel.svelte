<!-- TemplateChannel.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { templateChannels } from '@xivi/stores/template_store';
	import type { TemplateChannel } from '@xivi/data/template_entities';
	import type { ModalSettings } from '@skeletonlabs/skeleton';
	import { getModalStore } from '@skeletonlabs/skeleton';

	export let groupId: number;
	const modalStore = getModalStore();

	onMount(async () => {
		fetch(`/api/template/group/${groupId}/channels`)
			.then((response) => response.json())
			.then((data) => {
				console.log(data);
				templateChannels.set(data.templatechannels);
			})
			.catch((error) => {
				console.log(error);
				return [];
			});
	});

	async function updateSettings(channelId: number, formData: any) {
		if (formData.name != '' && formData.tvgid != '' && formData.logo != '') {
			let newSettings: TemplateChannel;
			newSettings = {
				id: $templateChannels[channelId].id,
				name: formData.name,
				tvgid: formData.tvgid,
				logoid: $templateChannels[channelId].logoid,
				uuid: $templateChannels[channelId].uuid,
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
			} catch (error) {
				console.log('Error updating template channel:', error);
				return;
			}
		}
	}

	function openSettings(channelId: number) {
		new Promise<boolean>((resolve) => {
			const modal: ModalSettings = {
				type: 'component',
				component: 'modalChannelSettings',
				meta: { channelId: channelId }
			};
			modalStore.trigger(modal);
		}).then((r: any) => {
			console.log('resolved response:', r);
			updateSettings(channelId, r);
		});
	}
</script>

<div class="table-container">
	<table class="table table-hover">
		<thead>
			<tr>
				<th>Logo</th>
				<th>Name</th>
				<th>tvg-id</th>
			</tr>
		</thead>
		<tbody>
			{#each $templateChannels as channel, i}
				<tr on:click={() => openSettings(i)}>
					<td><img class="w-14" src={channel.logo} alt="Logo" /></td>
					<td>{channel.name}</td>
					<td>{channel.tvgid}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
