<!-- Template.svelte -->
<script lang="ts">
    import TemplateGroup from './TemplateGroup.svelte';
    import { onMount } from 'svelte';
    import { Accordion, AccordionItem, popup, getModalStore, type PopupSettings, type ModalSettings } from '@skeletonlabs/skeleton';
    import { templates } from '@xivi/stores/template_store';
    import IconParkOutlineEditTwo from '~icons/icon-park-outline/edit-two'

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
                templates.set(data.templates);
            } catch (error) {
                console.log('Error creating template:', error);
                return [];
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
                return [];
            }
        }
    }
</script>

<div class="card card-hover p-2 px-2">
    <section class="flex items-center space-x-4">
        <h1>Templates</h1>
        <button class="btn btn-sm variant-ringed-primary" use:popup={templateSettings}>+ add new</button>
    </section>
    
    {#if $templates.length > 0}
        <Accordion>
            {#each $templates as template}
                <AccordionItem key={template.id}>
                    <svelte:fragment slot="lead">{template.id}</svelte:fragment>
                    <svelte:fragment slot="summary">
                        <div class="flex flex-row">
                            <h4>{template.name}</h4>
                            <button class="btn-icon btn-icon-sm !bg-transparent inset-y-0" on:click={() => renamePrompt(template.name, template.id)}><i><IconParkOutlineEditTwo/></i></button>
                        </div>
                    </svelte:fragment>
                    <svelte:fragment slot="content">
                        <TemplateGroup templateId={template.id} />
                    </svelte:fragment>
                </AccordionItem>
            {/each}
        </Accordion>
    {:else}
        <p>No templates found</p>
    {/if}
</div>
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