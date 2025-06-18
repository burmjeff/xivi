<!-- Viewer.svelte -->

<script lang="ts">
	import ViewerChannels from './ViewerChannels.svelte';
	import { onMount } from 'svelte';
	import { Accordion } from '@skeletonlabs/skeleton-svelte';
	import { templates } from '@xivi/stores/template_store';

	let selected = $state(0);

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
				if (response.ok) {
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

<section class="channels card flex h-full max-h-[90vh] flex-col justify-center p-1">
	<div class="flex-none">
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
					onchange={() => {
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
		<hr class="m-4 border-t-8! border-double!" />
	</div>

	<!-- Scrollable content section -->
	<div class="flex-1 overflow-y-auto px-2 pb-4">
		{#if $templates[selected] != null && $templates[selected].groups != null}
			<Accordion collapsible>
				{#if $templates[selected].groups.length > 0}
					{#each $templates[selected].groups as group, groupIdx (group.id)}
						<div class="groups w-content card mb-1 justify-center p-1 shadow-md">
							<Accordion.Item value={group.name}>
								{#snippet control()}
									<div class="flex w-full cursor-pointer flex-row items-center">
										<h4 class="flex-grow text-lg">{group.name}</h4>
									</div>
								{/snippet}
								{#snippet panel()}
									<ViewerChannels templateIdx={selected} groupId={group.id} {groupIdx} />
								{/snippet}
							</Accordion.Item>
						</div>
					{/each}
				{:else}
					<p>No groups found</p>
				{/if}
			</Accordion>
		{:else}
			<p>No groups found</p>
		{/if}
	</div>
</section>
