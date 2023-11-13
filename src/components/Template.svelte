<!-- Template.svelte -->
<script lang="ts">
    import TemplateGroup from './TemplateGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, getModalStore, type PopupSettings, type ModalSettings } from '@skeletonlabs/skeleton';
    import { templates } from '@xivi/stores/template_store';
    import IconParkOutlineEditTwo from '~icons/icon-park-outline/edit-two'
    import IconParkOutlineDelete from '~icons/icon-park-outline/delete';

    const modalStore = getModalStore();

    const updateTemplates = async () => {
        const response = await fetch('/api/templates');
        const data = await response.json();
        return data.templates;
    }

    onMount(async () => {
        templates.set(await updateTemplates());
        });
    
    let templateSettings: PopupSettings = {
        // Set the event as: click | hover | hover-click
        event: 'click',
        // Provide a matching 'data-popup' value.
        target: 'addTemplatePopup'
    };

    async function addTemplate() {
        const inputName = {name: (document.querySelector('.template_name input') as HTMLInputElement).value};
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
			body: 'Enter new template template name in field below.',
			value: templateName,
			valueAttr: { type: 'text', minlength: 1, maxlength: 20, required: true },
			response: (newName: string) => {
				if (newName) renameTemplate(newName, templateId);
			},
            buttonTextCancel: 'Cancel',
		    buttonTextSubmit: 'Submit',
		};
		modalStore.trigger(prompt);
	}

    async function renameTemplate(templateName: string, templateId: number) {
        if (templateName !=='') {
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
			},
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
			$templates = $templates.filter(t => t.id != templateId)
			modalStore.close();
		} catch (error) {
			console.log('Error deleting template:', error);
			return;
		}
	}
</script>

<section class="templates card card-hover p-1">
    <header class="templates-header flex justify-center items-center space-x-4">
        <h3 class="h3 font-bold">Templates</h3>
        <button class="btn btn-sm variant-ringed-primary" use:popup={templateSettings}>+ add new</button>
    </header>
    <div id="accord" class="templates-viewport min-w-full overflow-auto">
        {#if $templates.length > 0}
            <Accordion>
                {#each $templates as template, templateIdx (template.id)}
                    <AccordionItem class="card mb-1" key={template.id} bind:open={template.itemOpen}>
                        <svelte:fragment slot="summary">
                            <div class="flex flex-row">
                                <h4>{template.name}</h4>
                                <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" on:click={() => renamePrompt(template.name, template.id)}><i><IconParkOutlineEditTwo/></i></button>
                                <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" on:click={() => {template.itemOpen = true, deletePrompt(template.id)}}><i><IconParkOutlineDelete/></i></button>
                            </div>
                        </svelte:fragment>
                        <svelte:fragment slot="content">
                            <TemplateGroup templateId={template.id} templateIdx={templateIdx}/>
                        </svelte:fragment>
                    </AccordionItem>
                {/each}
            </Accordion>
        {:else}
            <p>No templates found</p>
        {/if}
    </div>
</section>
<div class="card p-4 gap-4" data-popup="addTemplatePopup">
	<h2>Add Template</h2>
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

<style>
    #accord {
        max-height: 82vh;
        height: 82vh;
    }
</style>