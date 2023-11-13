<!-- TemplateStatus.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { getModalStore } from '@skeletonlabs/skeleton';
	import { templates } from '@xivi/stores/template_store';
	import IconParkOutlineRefreshOne from '~icons/icon-park-outline/refresh-one';
	import { settings } from '@xivi/stores/settings_store';

	const modalStore = getModalStore();

	const updateTemplates = async () => {
		const response = await fetch('/api/templates');
		const data = await response.json();
		return data.templates;
	};

	const getSettings = async () => {
		const response = await fetch('/api/settings');
		const data = await response.json();
		return data.settings;
	};

	onMount(async () => {
		templates.set(await updateTemplates());
		settings.set(await getSettings());
	});

	async function refreshM3U(templateId: number) {
		try {
			const response = await fetch(`/api/m3u/${templateId}`, {
				method: 'POST'
			});
			const data = await response.status;
			console.log('Refreshed M3U:', data);
		} catch (error) {
			console.log('Error refreshing M3U:', error);
			return;
		}
	}
</script>

<section class="templates card card-hover p-1">
	<header class="templates-header flex justify-center items-center space-x-4">
		<h3 class="h3 font-bold">Templates</h3>
	</header>
	<div id="accord" class="templates-viewport min-w-full overflow-auto">
		{#if $templates.length > 0 && $settings != null}
			<div class="table-container">
				<!-- Native Table Element -->
				<table class="table table-hover">
					<thead>
						<tr>
							<th>Name</th>
							<th>M3U</th>
							<th>EPG</th>
							<th>Refresh</th>
						</tr>
					</thead>
					<tbody>
						{#each $templates as template, templateIdx (template.id)}
							<tr>
								<td>{template.name}</td>
								<td>http://{$settings.server.host}:{$settings.server.port}/m3u/{template.name}.m3u</td>
								<td>http://{$settings.server.host}:{$settings.server.port}/xmltv/{template.name}.xml</td>
								<td>
                                    <button class="btn-icon btn-icon-lg !bg-transparent inset-y-0"
                                    on:click={() => refreshM3U(template.id)}>
                                    <i><IconParkOutlineRefreshOne/></i>
                                    </button>
                                </td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else}
			<p>No templates found</p>
		{/if}
	</div>
</section>
