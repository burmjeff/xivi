<!-- Template.svelte -->
<script lang="ts">
	import TemplateGroup from './TemplateGroup.svelte';
	import TemplateSettings from './modals/TemplateSettings.svelte';
	import { onMount } from 'svelte';
	import {
		Accordion,
		Modal
	} from '@skeletonlabs/skeleton-svelte';
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
	import { templates } from '@xivi/stores/template_store';
	import Icon from '@iconify/svelte';
	import { fade } from 'svelte/transition';

	let TemplateModalOpen = $state(false);
	let deleteModalOpen = $state(false);
	let currentTemplateId = $state(0);
	let currentTemplateName = $state('');
	let isNewTemplate = $state(false);

	const updateTemplates = async () => {
		const response = await fetch('/api/templates');
		const data = await response.json();
		return data.templates;
	};

	onMount(async () => {
		templates.set(await updateTemplates());
	});

	// Floating UI state
	let templateSettingsOpen = $state(false);
	let addTemplateTooltipOpen = $state(false);
	let elemArrow: HTMLElement | null = $state(null);
	let accordionItem = $state<string[]>([]);

	// Floating UI setup for template settings
	const templateSettingsFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return templateSettingsOpen;
		},
		onOpenChange: (v) => {
			templateSettingsOpen = v;
		},
		placement: "top",
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		},
	});

	// Floating UI setup for add template tooltip
	const addTemplateTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return addTemplateTooltipOpen;
		},
		onOpenChange: (v) => {
			addTemplateTooltipOpen = v;
		},
		placement: "top",
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		},
	});

	// Interactions for add template tooltip
	const addTemplateTooltipRole = useRole(addTemplateTooltipFloating.context, { role: "tooltip" });
	const addTemplateTooltipHover = useHover(addTemplateTooltipFloating.context, { move: false });
	const addTemplateTooltipDismiss = useDismiss(addTemplateTooltipFloating.context);
	const addTemplateTooltipInteractions = useInteractions([addTemplateTooltipRole, addTemplateTooltipHover, addTemplateTooltipDismiss]);

	function modalTemplate(isNew: boolean, id: number, name: string) {
		isNewTemplate = isNew;
		currentTemplateId = id;
		currentTemplateName = name;
		TemplateModalOpen = true;
	}

	function handleTemplateClose(formData: any = null) {
		console.log('handleTemplateClose called with formData:', formData);
		if (formData) {
			addTemplate(formData, isNewTemplate, currentTemplateId);
		}
		TemplateModalOpen = false;
	}

	async function addTemplate(formData: any, isNew: boolean, id: number) {
		if (formData.name) {
			const newTemplate = {
				id: id,
				name: formData.name
			};

			try {
				let method: string;
				if (isNew) {
					method = 'POST';
				} else {
					method = 'PUT';
				}

				const response = await fetch('/api/template', {
					method: method,
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newTemplate)
				});
				if (response.ok) {
					if (isNew) {
						const data = await response.json();
						console.log('Createdtemplate:', data);
						$templates.push(data.template);
						$templates = [...$templates];
					} else {
						console.log('Updated template: ', newTemplate.name);
						$templates = $templates.map((template) => {
							if (template.id === id) {
								return {
									...template,
									name: formData.name
								};
							}
							return template;
						});
						$templates = [...$templates];
					}
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error creating playlist:', error);
			}
		}
	}

	function deletePrompt(templateId: number) {
		currentTemplateId = templateId;
		deleteModalOpen = true;
	}

	function handleDeleteClose(confirm: boolean) {
		if (confirm) {
			deleteTemplate(currentTemplateId);
		}
		deleteModalOpen = false;
	}

	async function deleteTemplate(templateId: number) {
		try {
			const response = await fetch(`/api/template/${templateId}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted template:', data);
			$templates = $templates.filter((t) => t.id != templateId);
		} catch (error) {
			console.log('Error deleting template:', error);
			return;
		}
	}
</script>

<section class="templates card card-hover p-1">
	<header class="templates-header flex items-center justify-center">
		<h3 class="h3 font-bold">Templates</h3>
		<button
			class="btn btn-md"
			onclick={() => modalTemplate(true, 0, '')}
			bind:this={addTemplateTooltipFloating.elements.reference}
			{...addTemplateTooltipInteractions.getReferenceProps()}
		>
			<Icon icon="icon-park-twotone:add-one" color="#0a7e85" width="25" height="25" />
			{#if addTemplateTooltipOpen}
				<div
					bind:this={addTemplateTooltipFloating.elements.floating}
					style={addTemplateTooltipFloating.floatingStyles}
					{...addTemplateTooltipInteractions.getFloatingProps()}
					class="floating popover-neutral card p-2"
					transition:fade={{ duration: 200 }}
				>
					<p>Add New Template</p>
					<FloatingArrow bind:ref={elemArrow} context={addTemplateTooltipFloating.context} fill="#575969" />
				</div>
			{/if}
		</button>
	</header>
	<div id="accord" class="templates-viewport min-w-full overflow-auto">
		{#if $templates.length > 0}
			<Accordion value={accordionItem} onValueChange={(e) => (accordionItem = e.value)} collapsible>
				{#each $templates as template, templateIdx (template.id)}
					<div class="card shadow-md mb-1">
						<Accordion.Item value={template.name} >
							{#snippet control()}
								<div class="flex flex-row items-center w-full cursor-pointer">
									<h4 class="text-lg flex-grow">{template.name}</h4>
									<div class="flex flex-row gap-2">
										<button
											class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
											onclick={(e) => {
												modalTemplate(false, template.id, template.name);
												e.stopPropagation();
											}}
										>
											<Icon icon="icon-park-outline:edit-two" width="18" height="18" />
										</button>
										<button
											class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
											onclick={(e) => {
												deletePrompt(template.id)
												e.stopPropagation();
											}}
										>
											<Icon icon="icon-park-outline:delete" width="18" height="18" />
										</button>
									</div>
								</div>
							{/snippet}
							{#snippet panel()}
								<TemplateGroup templateId={template.id} {templateIdx} />
							{/snippet}
						</Accordion.Item>
					</div>
				{/each}
			</Accordion>
		{:else}
			<p>No templates found</p>
		{/if}
	</div>
</section>

<Modal
	open={TemplateModalOpen}
	onOpenChange={(e) => (TemplateModalOpen = e.open)}
	triggerBase="btn preset-tonal"
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl max-w-screen-sm"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet trigger()}{/snippet}
	{#snippet content()}
		<TemplateSettings
			parent={{ onClose: handleTemplateClose }}
			isNew={isNewTemplate}
			id={currentTemplateId}
			name={currentTemplateName}
		/>
	{/snippet}
</Modal>

<Modal
	open={deleteModalOpen}
	onOpenChange={(e) => (deleteModalOpen = e.open)}
	triggerBase="btn preset-tonal"
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl max-w-screen-sm"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet trigger()}{/snippet}
	{#snippet content()}
		<header class="text-2xl font-bold">Please Confirm</header>
		<article>Are you sure you wish to delete this template?</article>
		<footer class="flex justify-end space-x-2">
			<button class="btn preset-outlined-surface-500" onclick={() => handleDeleteClose(false)}>Cancel</button>
			<button class="btn preset-tonal-error" onclick={() => handleDeleteClose(true)}>Delete</button>
		</footer>
	{/snippet}
</Modal>

<style>
	#accord {
		max-height: 76vh;
		height: 76vh;
	}
</style>
