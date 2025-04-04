<!-- TemplateStatus.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { Switch, Modal } from '@skeletonlabs/skeleton-svelte';
	import { templates } from '@xivi/stores/template_store';
	import Icon from '@iconify/svelte';
	import { settings } from '@xivi/stores/settings_store';
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
	import { fade } from 'svelte/transition';

	// Modal state
	let modalOpen = $state(false);
	let modalContent = $state('');

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

	// Floating UI state
	let refreshTooltipOpen = $state(false);
	let elemArrow: HTMLElement | null = $state(null);

	// Floating UI setup for refresh tooltip
	const refreshTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return refreshTooltipOpen;
		},
		onOpenChange: (v) => {
			refreshTooltipOpen = v;
		},
		placement: "right",
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		},
	});

	// Interactions for refresh tooltip
	const refreshTooltipRole = useRole(refreshTooltipFloating.context, { role: "tooltip" });
	const refreshTooltipHover = useHover(refreshTooltipFloating.context, { move: false });
	const refreshTooltipDismiss = useDismiss(refreshTooltipFloating.context);
	const refreshTooltipInteractions = useInteractions([refreshTooltipRole, refreshTooltipHover, refreshTooltipDismiss]);

	async function refreshM3U(templateId: number, templateName: string) {
		try {
			const response = await fetch(`/api/m3u/${templateId}`, {
				method: 'POST'
			});
			const data = await response.status;
			console.log('Refreshed M3U:', data);

			// Show success modal
			modalContent = `Successfully refreshed M3U and EPG for template "${templateName}".`;
			modalOpen = true;
		} catch (error) {
			console.log('Error refreshing M3U:', error);

			// Show error modal
			modalContent = `Error refreshing M3U and EPG for template "${templateName}". Please try again.`;
			modalOpen = true;
			return;
		}
	}

	function closeModal() {
		modalOpen = false;
	}

</script>

<section class="templates card card-hover p-1">
	<header class="templates-header flex items-center justify-center space-x-4">
		<h3 class="h3 font-bold">Status List</h3>
	</header>
	<div id="accord" class="templates-viewport min-w-full overflow-auto">
		{#if $templates.length > 0 && $settings != null}
			<div class="table-container">
				<!-- Native Table Element -->
				<table class="table ">
					<thead>
						<tr id="thead">
							<th>Name</th>
							<th>M3U</th>
							<th>EPG</th>
							<th>Refresh</th>
						</tr>
					</thead>
					<tbody>
						{#each $templates as template, templateIdx (template.id)}
							<tr id="thead">
								<td>{template.name}</td>
								<td
									>http://{$settings.server.host}:{$settings.server
										.port}/m3u/{template.name}.m3u</td
								>
								<td
									>http://{$settings.server.host}:{$settings.server
										.port}/xmltv/{template.name}.xml</td
								>
								<td>
									<button
										class="btn-icon btn-icon-lg inset-y-0 bg-transparent!"
										onclick={() => refreshM3U(template.id, template.name)}
										bind:this={refreshTooltipFloating.elements.reference}
										{...refreshTooltipInteractions.getReferenceProps()}
									>
										<Icon icon="icon-park-outline:refresh-one" width="25" height="25" />
										{#if refreshTooltipOpen}
											<div
												bind:this={refreshTooltipFloating.elements.floating}
												style={refreshTooltipFloating.floatingStyles}
												{...refreshTooltipInteractions.getFloatingProps()}
												class="floating popover-neutral card p-2"
												transition:fade={{ duration: 200 }}
											>
												<p>Regenerate m3u and EPG</p>
												<FloatingArrow bind:ref={elemArrow} context={refreshTooltipFloating.context} fill="#575969" />
											</div>
										{/if}
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

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	triggerBase="btn preset-tonal"
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl max-w-screen-sm"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
	<div class="card p-4 w-modal shadow-xl space-y-4">
		<header class="text-2xl font-bold">Notification</header>
		<article>{modalContent}</article>
		<footer class="flex justify-end space-x-2">
			<button class="btn preset-outlined-surface-500" onclick={closeModal}>Close</button>
		</footer>
	</div>
	{/snippet}
</Modal>

<style>
	#thead {
		position: relative;
		text-align: center;
	}
</style>
