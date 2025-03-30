<!-- Template.svelte -->
<script lang="ts">
	import TemplateGroup from './TemplateGroup.svelte';
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
		useClick,
		useInteractions,
		useRole,
	} from "@skeletonlabs/floating-ui-svelte";
	import { templates } from '@xivi/stores/template_store';
	import Icon from '@iconify/svelte';
	import { fade } from 'svelte/transition';

	let renameModalOpen = $state(false);
	let deleteModalOpen = $state(false);
	let currentTemplateId = $state(0);
	let currentTemplateName = $state('');
	let renameInputValue = $state('');

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

	// Interactions for template settings
	const templateSettingsRole = useRole(templateSettingsFloating.context);
	const templateSettingsClick = useClick(templateSettingsFloating.context);
	const templateSettingsDismiss = useDismiss(templateSettingsFloating.context);
	const templateSettingsInteractions = useInteractions([templateSettingsRole, templateSettingsClick, templateSettingsDismiss]);

	// Interactions for add template tooltip
	const addTemplateTooltipRole = useRole(addTemplateTooltipFloating.context, { role: "tooltip" });
	const addTemplateTooltipHover = useHover(addTemplateTooltipFloating.context, { move: false });
	const addTemplateTooltipDismiss = useDismiss(addTemplateTooltipFloating.context);
	const addTemplateTooltipInteractions = useInteractions([addTemplateTooltipRole, addTemplateTooltipHover, addTemplateTooltipDismiss]);

	async function addTemplate() {
		const inputName = {
			name: (document.querySelector('.template_name input') as HTMLInputElement).value
		};
		if (inputName !== null) {
			try {
				const response = await fetch('/api/template', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(inputName)
				});
				const data = await response.json();
				console.log('Created template', data);
				$templates.push(data.template);
				$templates = $templates;
			} catch (error) {
				console.log('Error creating template:', error);
			}
		}
	}

	function renamePrompt(templateName: string, templateId: number): void {
		currentTemplateId = templateId;
		currentTemplateName = templateName;
		renameInputValue = templateName;
		renameModalOpen = true;
	}

	function handleRenameClose(confirm: boolean) {
		if (confirm && renameInputValue) {
			renameTemplate(renameInputValue, currentTemplateId);
		}
		renameModalOpen = false;
	}

	async function renameTemplate(templateName: string, templateId: number) {
		if (templateName !== '') {
			const newTemplate = {
				id: templateId,
				name: templateName
			};
			try {
				const response = await fetch(`/api/template`, {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newTemplate)
				});
				if (response.ok) {
					templates.set(await updateTemplates());
				} else {
					console.error('Error:', response.status, response.statusText);
				}
			} catch (error) {
				console.log('Error updating template:', error);
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

<Modal
	open={renameModalOpen}
	onOpenChange={(e) => (renameModalOpen = e.open)}
	contentBase="card bg-surface-100-900 p-4 shadow-xl max-w-screen-sm"
	positionerBase="fixed inset-0 flex justify-center items-center"
	backdropClasses="backdrop-blur-sm fixed inset-0"
>
	{#snippet content()}
		<header class="text-2xl font-bold">Rename Template</header>
		<article>
			<label class="label">
				<span>Enter new template name</span>
				<input
					class="input"
					type="text"
					bind:value={renameInputValue}
					minlength="1"
					maxlength="20"
					required
				/>
			</label>
		</article>
		<footer class="flex justify-end gap-4">
			<button type="button" class="btn preset-outlined-surface-500" onclick={() => handleRenameClose(false)}>Cancel</button>
			<button type="button" class="btn preset-filled-primary-500" onclick={() => handleRenameClose(true)}>Submit</button>
		</footer>
	{/snippet}
</Modal>

<Modal
	open={deleteModalOpen}
	onOpenChange={(e) => (deleteModalOpen = e.open)}
	contentBase="card bg-surface-100-900 p-4 shadow-xl max-w-screen-sm"
	positionerBase="fixed inset-0 flex justify-center items-center"
	backdropClasses="backdrop-blur-sm fixed inset-0"
>
	{#snippet content()}
		<header class="text-2xl font-bold">Please Confirm</header>
		<article>Are you sure you wish to delete this template?</article>
		<footer class="flex justify-end space-x-2">
			<button class="btn preset-outlined-surface-500" onclick={() => handleDeleteClose(false)}>Cancel</button>
			<button class="btn preset-tonal-error" onclick={() => handleDeleteClose(true)}>Delete</button>
		</footer>
	{/snippet}
</Modal>

<section class="templates card card-hover p-1">
	<header class="templates-header flex items-center justify-center">
		<h3 class="h3 font-bold">Templates</h3>
		<button
			class="btn btn-md"
			bind:this={templateSettingsFloating.elements.reference}
			{...templateSettingsInteractions.getReferenceProps()}
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
			<Accordion collapsible>
				{#each $templates as template, templateIdx (template.id)}
					<div class="card shadow-md mb-1">
						<Accordion.Item value={template.name} >
							{#snippet control()}
									<div class="item-center flex flex-row">
										<h4 class="text-lg">{template.name}</h4>
										<button
											class="btn-icon btn-icon-md inset-y-0 bg-transparent! ml-auto"
											onclick={(e) => {
												renamePrompt(template.name, template.id)
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
{#if templateSettingsOpen}
<div
	bind:this={templateSettingsFloating.elements.floating}
	style={templateSettingsFloating.floatingStyles}
	{...templateSettingsInteractions.getFloatingProps()}
	class="floating popover-neutral card gap-4 p-4"
	transition:fade={{ duration: 200 }}
>
	<header class="justify-center text-center text-2xl font-bold">Add Template</header>
	<div class="space-y-4">
		<label class="template_name">
			<span>Template Name</span>
			<input class="input" type="text" placeholder="Template Name" />
		</label>
		<label class="submit_button">
			<button class="btn bg-primary-500" onclick={addTemplate}>Add Template</button>
		</label>
	</div>
	<FloatingArrow bind:ref={elemArrow} context={templateSettingsFloating.context} fill="#575969" />
</div>
{/if}

<style>
	#accord {
		max-height: 76vh;
		height: 76vh;
	}
</style>
