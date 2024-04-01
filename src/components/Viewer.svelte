<!-- Viewer.svelte -->

<script lang="ts">
	import ViewerChannels from './ViewerChannels.svelte';
	import { onMount } from 'svelte';
	import { Accordion, AccordionItem } from '@skeletonlabs/skeleton';
	import { templates } from '@xivi/stores/template_store';

	let selected = 0;

	const updateTemplates = async () => {
		const response = await fetch('/api/templates');
		const data = await response.json();
		return data.templates;
	};

	onMount(async () => {
		const fetchedData = await updateTemplates();
		if (typeof fetchedData !== 'undefined') {
			templates.set(fetchedData);
			getGroups();
		}
	});

	async function getGroups() {
		if (
			typeof $templates[selected] !== 'undefined' &&
			typeof $templates[selected].groups === 'undefined'
		) {
			try {
				const response = await fetch(`/api/template/${$templates[selected].id}/groups`);
				if (await response.ok) {
					const data = await response.json();
					if (typeof data !== 'undefined') {
						$templates[selected].groups = data.templategroups;
						$templates[selected].groups = $templates[selected].groups;
					}
				} else console.log('Error getting template groups:', response);
			} catch (error) {
				console.log('Error getting template groups:', error);
			}
		}
	}
</script>

<section class="channels card justify-center p-1">
	<header class="channels-header flex items-center justify-center space-x-4">
		<h3 class="h3 font-bold">Stream Viewer</h3>
	</header>
	<div
		id="selector"
		class="m-2 grid w-fit grid-cols-2 items-center justify-center space-x-4 text-center"
	>
		<div>Select Template:</div>
		{#if $templates.length > 0}
			<select
				class="select"
				bind:value={selected}
				on:change={() => {
					getGroups();
				}}
			>
				{#each $templates as template, templateIdx (template.id)}
					<option value={templateIdx}>{template.name}</option>
				{/each}
			</select>
		{:else}
			<p>No templates found</p>
		{/if}
	</div>
	<hr class="m-4 !border-t-8 !border-double" />
	{#if $templates[selected] != null && $templates[selected].groups != null}
		<Accordion>
			{#if $templates[selected].groups.length > 0}
				{#each $templates[selected].groups as group, groupIdx (group.id)}
					<div class="groups w-content justify-center p-1 card shadow-md mb-1">
						<AccordionItem class="mb-1" key={groupIdx} bind:open={group.itemOpen}>
							<svelte:fragment slot="summary">
								<div class="flex flex-row items-center">
									<h4 class="text-lg">{group.name}</h4>
								</div>
							</svelte:fragment>
							<svelte:fragment slot="content">
								<ViewerChannels templateIdx={selected} groupId={group.id} {groupIdx} />
							</svelte:fragment>
						</AccordionItem>
					</div>
				{/each}
			{:else}
				<p>No groups found</p>
			{/if}
		</Accordion>
	{:else}
		<p>No groups found</p>
	{/if}
</section>
