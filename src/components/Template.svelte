<!-- Template.svelte -->
<script lang="ts">
	import TemplateGroup from './TemplateGroup.svelte';
	import { onMount } from 'svelte';
	import {
		Accordion,
		AccordionItem,
		popup,
		getModalStore,
		type PopupSettings,
		type ModalSettings
	} from '@skeletonlabs/skeleton';
	import { templates } from '@xivi/stores/template_store';
	import Icon from '@iconify/svelte';

	const modalStore = getModalStore();

	const updateTemplates = async () => {
		const response = await fetch('/api/templates');
		const data = await response.json();
		return data.templates;
	};

	onMount(async () => {
		templates.set(await updateTemplates());
	});

	let templateSettings: PopupSettings = {
		// Set the event as: click | hover | hover-click
		event: 'click',
		// Provide a matching 'data-popup' value.
		target: 'addTemplatePopup'
	};

	const addTemplateTooltip: PopupSettings = {
		event: 'hover',
		target: 'addTemplateTooltip',
		placement: 'top'
	};

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
		const prompt: ModalSettings = {
			type: 'prompt',
			title: 'Rename Template',
			body: 'Enter new template name in field below.',
			value: templateName,
			valueAttr: { type: 'text', minlength: 1, maxlength: 20, required: true },
			response: (newName: string) => {
				if (newName) renameTemplate(newName, templateId);
			},
			buttonTextCancel: 'Cancel',
			buttonTextSubmit: 'Submit'
		};
		modalStore.trigger(prompt);
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
		const modal: ModalSettings = {
			type: 'confirm',
			title: 'Please Confirm',
			body: 'Are you sure you wish to delete this template?',
			// TRUE if confirm pressed, FALSE if cancel pressed
			response: (r: boolean) => {
				if (r) deleteTemplate(templateId);
			}
		};
		modalStore.trigger(modal);
	}

	async function deleteTemplate(templateId: number) {
		try {
			const response = await fetch(`/api/template/${templateId}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted template:', data);
			$templates = $templates.filter((t) => t.id != templateId);
			modalStore.close();
		} catch (error) {
			console.log('Error deleting template:', error);
			return;
		}
	}
</script>

<section class="templates card card-hover p-1">
	<header class="templates-header flex items-center justify-center">
		<h3 class="h3 font-bold">Templates</h3>
		<button class="btn btn-md" use:popup={templateSettings} use:popup={addTemplateTooltip}>
			<Icon icon="icon-park-twotone:add-one" color="#0a7e85" width="25" height="25" />
		</button>
	</header>
	<div id="accord" class="templates-viewport min-w-full overflow-auto">
		{#if $templates.length > 0}
			<Accordion>
				{#each $templates as template, templateIdx (template.id)}
					<AccordionItem class="card mb-1" key={template.id} bind:open={template.itemOpen}>
						<svelte:fragment slot="summary">
							<div class="item-center flex flex-row">
								<h4 class="text-lg">{template.name}</h4>
								<button
									class="btn-icon btn-icon-sm inset-y-0 !bg-transparent ml-auto"
									on:click={() => renamePrompt(template.name, template.id)}
								>
									<Icon icon="icon-park-outline:edit-two" width="18" height="18" />
								</button>
								<button
									class="btn-icon btn-icon-sm inset-y-0 !bg-transparent"
									on:click={() => {
										(template.itemOpen = true), deletePrompt(template.id);
									}}
								>
									<Icon icon="icon-park-outline:delete" width="18" height="18" />
								</button>
							</div>
						</svelte:fragment>
						<svelte:fragment slot="content">
							<TemplateGroup templateId={template.id} {templateIdx} />
						</svelte:fragment>
					</AccordionItem>
				{/each}
			</Accordion>
		{:else}
			<p>No templates found</p>
		{/if}
	</div>
</section>
<div class="card gap-4 p-4" data-popup="addTemplatePopup">
	<header class="justify-center text-center text-2xl font-bold">Add Template</header>
	<div class="space-y-4">
		<label class="template_name">
			<span>Template Name</span>
			<input class="input" type="text" placeholder="Template Name" />
		</label>
		<label class="submit_button">
			<button class="btn bg-primary-500" on:click={addTemplate}>Add Template</button>
		</label>
	</div>
</div>

<div class="card variant-filled-secondary p-2" data-popup="addTemplateTooltip">
	<p>Add New Template</p>
	<div class="variant-filled-secondary arrow" />
</div>

<style>
	#accord {
		max-height: 82vh;
		height: 82vh;
	}
</style>
