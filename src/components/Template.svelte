<!-- Template.svelte -->
<script lang="ts">
	import TemplateGroup from './TemplateGroup.svelte';
	import TemplateSettings from './modals/TemplateSettings.svelte';
	import { onMount } from 'svelte';
	import { Accordion } from '@skeletonlabs/skeleton-svelte';
	import Modal from '@xivi/components/Modal.svelte';
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
		useRole
	} from '@skeletonlabs/floating-ui-svelte';
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
	let addTemplateTooltipOpen = $state(false);
	let editTooltipOpen = $state<{ [key: number]: boolean }>({});
	let deleteTooltipOpen = $state<{ [key: number]: boolean }>({});
	let addTemplateElemArrow: HTMLElement | null = $state(null);
	let accordionItem = $state<string[]>([]);

	// Floating UI setup for add template tooltip
	const addTemplateTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return addTemplateTooltipOpen;
		},
		onOpenChange: (v) => {
			addTemplateTooltipOpen = v;
		},
		placement: 'top',
		get middleware() {
			return [offset(10), flip(), addTemplateElemArrow && arrow({ element: addTemplateElemArrow })];
		}
	});

	// Interactions for add template tooltip
	const addTemplateTooltipRole = useRole(addTemplateTooltipFloating.context, { role: 'tooltip' });
	const addTemplateTooltipHover = useHover(addTemplateTooltipFloating.context, { move: false });
	const addTemplateTooltipDismiss = useDismiss(addTemplateTooltipFloating.context);
	const addTemplateTooltipInteractions = useInteractions([
		addTemplateTooltipRole,
		addTemplateTooltipHover,
		addTemplateTooltipDismiss
	]);

	// Helper function to create floating UI for template buttons
	function createTooltipFloating(templateId: number, tooltipType: 'edit' | 'delete') {
		const tooltipState = tooltipType === 'edit' ? editTooltipOpen : deleteTooltipOpen;

		return useFloating({
			whileElementsMounted: autoUpdate,
			get open() {
				return tooltipState[templateId] || false;
			},
			onOpenChange: (v) => {
				tooltipState[templateId] = v;
			},
			placement: 'top',
			get middleware() {
				return [offset(10), flip()];
			}
		});
	}

	// Helper function to create interactions for template tooltips
	function createTooltipInteractions(floating: any) {
		const role = useRole(floating.context, { role: 'tooltip' });
		const hover = useHover(floating.context, { move: false });
		const dismiss = useDismiss(floating.context);
		return useInteractions([role, hover, dismiss]);
	}

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

<section class="templates h-full w-full p-1">
	<header
		class="templates-header border-surface-700/30 flex items-center justify-center border-b p-1"
	>
		<h4 class="h4 text-primary-400 font-bold">Templates</h4>
		<button
			class="btn btn-md self-start"
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
					class="floating glass card p-2 shadow-lg"
					transition:fade={{ duration: 200 }}
				>
					<p class="text-sm font-medium"><strong>Create a new template</strong></p>
					<FloatingArrow
						bind:ref={addTemplateElemArrow}
						context={addTemplateTooltipFloating.context}
						fill="#1e293b"
					/>
				</div>
			{/if}
		</button>
	</header>
	<div id="accord" class="templates-viewport min-w-full overflow-auto">
		{#if $templates.length > 0}
			<Accordion value={accordionItem} onValueChange={(e) => (accordionItem = e.value)} collapsible>
				{#each $templates as template, templateIdx (template.id)}
					{@const editFloating = createTooltipFloating(template.id, 'edit')}
					{@const editInteractions = createTooltipInteractions(editFloating)}
					{@const deleteFloating = createTooltipFloating(template.id, 'delete')}
					{@const deleteInteractions = createTooltipInteractions(deleteFloating)}
					<div class="card mb-1 shadow-md">
						<Accordion.Item value={template.name}>
							<Accordion.ItemTrigger class="flex w-full cursor-pointer flex-row items-center p-4">
								<h4 class="flex-grow text-left align-middle text-lg">{template.name}</h4>
								<div class="flex flex-row gap-1">
									<button
										class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
										onclick={(e) => {
											modalTemplate(false, template.id, template.name);
											e.stopPropagation();
										}}
										bind:this={editFloating.elements.reference}
										{...editInteractions.getReferenceProps()}
									>
										<Icon icon="icon-park-outline:edit-two" width="18" height="18" />
										{#if editTooltipOpen[template.id]}
											<div
												bind:this={editFloating.elements.floating}
												style={editFloating.floatingStyles}
												{...editInteractions.getFloatingProps()}
												class="floating glass card p-2 shadow-lg"
												transition:fade={{ duration: 200 }}
											>
												<p class="text-sm font-medium"><strong>Edit Template</strong></p>
											</div>
										{/if}
									</button>
									<button
										class="btn-icon btn-icon-md inset-y-0 bg-transparent!"
										onclick={(e) => {
											deletePrompt(template.id);
											e.stopPropagation();
										}}
										bind:this={deleteFloating.elements.reference}
										{...deleteInteractions.getReferenceProps()}
									>
										<Icon icon="icon-park-outline:delete" width="18" height="18" />
										{#if deleteTooltipOpen[template.id]}
											<div
												bind:this={deleteFloating.elements.floating}
												style={deleteFloating.floatingStyles}
												{...deleteInteractions.getFloatingProps()}
												class="floating glass card p-2 shadow-lg"
												transition:fade={{ duration: 200 }}
											>
												<p class="text-sm font-medium"><strong>Delete Template</strong></p>
											</div>
										{/if}
									</button>
								</div>
							</Accordion.ItemTrigger>
							<Accordion.ItemContent>
								<TemplateGroup templateId={template.id} {templateIdx} />
							</Accordion.ItemContent>
						</Accordion.Item>
					</div>
				{/each}
			</Accordion>
		{:else}
			<div class="flex flex-col items-center justify-center px-2 py-2 text-center">
				<Icon icon="mdi:folder-off" class="text-surface-500 mb-3" width="48" height="48" />
				<h3 class="text-surface-300 mb-2 text-xl font-medium">No Templates Found</h3>
				<p class="text-surface-400 max-w-md">
					Create your first template by clicking the "Create Template" button above.
				</p>
			</div>
		{/if}
	</div>
</section>

<Modal
	open={TemplateModalOpen}
	onOpenChange={(e) => (TemplateModalOpen = e.open)}
	contentBase="card bg-surface-100-900 shadow-xl"
	backdropClasses="backdrop-blur-sm"
>
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
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
		<header class="text-2xl font-bold">Please Confirm</header>
		<article>Are you sure you wish to delete this template?</article>
		<footer class="flex justify-end space-x-2">
			<button class="btn preset-outlined-surface-500" onclick={() => handleDeleteClose(false)}
				>Cancel</button
			>
			<button class="btn preset-tonal-error" onclick={() => handleDeleteClose(true)}>Delete</button>
		</footer>
	{/snippet}
</Modal>

<style>
	#accord {
		max-height: 72vh;
		height: 72vh;
	}
</style>
