<!-- TemplateStatus.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { Modal } from '@skeletonlabs/skeleton-svelte';
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
			const data = response.status;
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

<section class="templates card bg-surface-800/30 border border-surface-700/50 shadow-lg rounded-lg p-6">
	<header class="templates-header flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold text-primary-300">Template Status</h2>
			<p class="text-surface-300 text-sm mt-1">Manage your templates and access their M3U and EPG files</p>
		</div>

		<div class="flex items-center gap-2">
			<span class="text-sm text-surface-300">{$templates.length} Templates</span>
		</div>
	</header>

	<div id="accord" class="templates-viewport overflow-auto rounded-lg border border-surface-700/30">
		{#if $templates.length > 0 && $settings != null}
			<div class="table-container">
				<!-- Enhanced Table Element -->
				<table class="table w-full">
					<thead>
						<tr class="bg-surface-700/30 border-b border-surface-600/30">
							<th class="py-3 px-4 text-left font-medium text-surface-200">Template Name</th>
							<th class="py-3 px-4 text-left font-medium text-surface-200">M3U URL</th>
							<th class="py-3 px-4 text-left font-medium text-surface-200">EPG URL</th>
							<th class="py-3 px-4 text-center font-medium text-surface-200 w-24">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each $templates as template (template.id)}
							<tr class="border-b border-surface-700/20 hover:bg-surface-700/10 transition-colors duration-150">
								<td class="py-4 px-4 font-medium text-primary-400">{template.name}</td>
								<td class="py-4 px-4 text-surface-300">
									<div class="flex items-center gap-2">
										<span class="truncate">http://{$settings.server.host}:{$settings.server.port}/m3u/{template.name}.m3u</span>
										<button
											class="text-primary-400 hover:text-primary-300 transition-colors"
											onclick={() => {
												navigator.clipboard.writeText(`http://${$settings.server.host}:${$settings.server.port}/m3u/${template.name}.m3u`);
												// Could add a toast notification here
											}}
										>
											<Icon icon="mdi:content-copy" width="16" height="16" />
										</button>
									</div>
								</td>
								<td class="py-4 px-4 text-surface-300">
									<div class="flex items-center gap-2">
										<span class="truncate">http://{$settings.server.host}:{$settings.server.port}/xmltv/{template.name}.xml</span>
										<button
											class="text-primary-400 hover:text-primary-300 transition-colors"
											onclick={() => {
												navigator.clipboard.writeText(`http://${$settings.server.host}:${$settings.server.port}/xmltv/${template.name}.xml`);
												// Could add a toast notification here
											}}
										>
											<Icon icon="mdi:content-copy" width="16" height="16" />
										</button>
									</div>
								</td>
								<td class="py-4 px-4 text-center">
									<button
										class="btn bg-primary-700 hover:bg-primary-600 text-white p-2 rounded-lg transition-colors"
										onclick={() => refreshM3U(template.id, template.name)}
										bind:this={refreshTooltipFloating.elements.reference}
										{...refreshTooltipInteractions.getReferenceProps()}
									>
										<Icon icon="mdi:refresh" width="18" height="18" />
										{#if refreshTooltipOpen}
											<div
												bind:this={refreshTooltipFloating.elements.floating}
												style={refreshTooltipFloating.floatingStyles}
												{...refreshTooltipInteractions.getFloatingProps()}
												class="floating glass card p-3 shadow-lg"
												transition:fade={{ duration: 200 }}
											>
												<p class="text-sm font-medium">Regenerate M3U and EPG files</p>
												<FloatingArrow bind:ref={elemArrow} context={refreshTooltipFloating.context} fill="#1e293b" />
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
			<div class="flex flex-col items-center justify-center py-12 px-4 text-center">
				<Icon icon="mdi:playlist-remove" class="text-surface-500 mb-4" width="48" height="48" />
				<h3 class="text-xl font-medium text-surface-300 mb-2">No Templates Found</h3>
				<p class="text-surface-400 max-w-md">Create your first template to get started with Xivi.</p>
			</div>
		{/if}
	</div>
</section>

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	triggerBase="btn preset-tonal"
	contentBase="card bg-surface-800 p-0 shadow-xl max-w-screen-sm border border-surface-700/50 rounded-lg overflow-hidden"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
	<div class="w-modal">
		<header class="bg-surface-700/30 p-4 border-b border-surface-700/30">
			<h3 class="text-xl font-bold text-primary-300 flex items-center gap-2">
				<Icon icon="mdi:information-outline" width="24" height="24" />
				<span>Notification</span>
			</h3>
		</header>

		<div class="p-6">
			<p class="text-surface-200">{modalContent}</p>
		</div>

		<footer class="bg-surface-700/20 p-4 border-t border-surface-700/30 flex justify-end">
			<button
				class="btn bg-primary-700 hover:bg-primary-600 text-white px-4 py-2 rounded-lg transition-colors duration-200 flex items-center gap-2"
				onclick={closeModal}
			>
				<Icon icon="mdi:check" width="18" height="18" />
				<span>Close</span>
			</button>
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
