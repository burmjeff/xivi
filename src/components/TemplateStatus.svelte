<!-- TemplateStatus.svelte -->
<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '@xivi/components/Modal.svelte';
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
		useRole
	} from '@skeletonlabs/floating-ui-svelte';
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
		placement: 'right',
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		}
	});

	// Interactions for refresh tooltip
	const refreshTooltipRole = useRole(refreshTooltipFloating.context, { role: 'tooltip' });
	const refreshTooltipHover = useHover(refreshTooltipFloating.context, { move: false });
	const refreshTooltipDismiss = useDismiss(refreshTooltipFloating.context);
	const refreshTooltipInteractions = useInteractions([
		refreshTooltipRole,
		refreshTooltipHover,
		refreshTooltipDismiss
	]);

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

<section
	class="templates card bg-surface-800/30 border-surface-700/50 rounded-lg border p-6 shadow-lg"
>
	<header class="templates-header mb-6 flex items-center justify-between">
		<div>
			<h2 class="text-primary-300 text-2xl font-bold">Template Status</h2>
			<p class="text-surface-300 mt-1 text-sm">
				Manage your templates and access their M3U and EPG files
			</p>
		</div>

		<div class="flex items-center gap-2">
			<span class="text-surface-300 text-sm">{$templates.length} Templates</span>
		</div>
	</header>

	<div id="accord" class="templates-viewport border-surface-700/30 overflow-auto rounded-lg border">
		{#if $templates.length > 0 && $settings != null}
			<div class="table-container">
				<!-- Enhanced Table Element -->
				<table class="table w-full">
					<thead>
						<tr class="bg-surface-700/30 border-surface-600/30 border-b">
							<th class="text-surface-200 px-4 py-3 text-left font-medium">Template Name</th>
							<th class="text-surface-200 px-4 py-3 text-left font-medium">M3U URL</th>
							<th class="text-surface-200 px-4 py-3 text-left font-medium">EPG URL</th>
							<th class="text-surface-200 w-24 px-4 py-3 text-center font-medium">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each $templates as template (template.id)}
							<tr
								class="border-surface-700/20 hover:bg-surface-700/10 border-b transition-colors duration-150"
							>
								<td class="text-primary-400 px-4 py-4 font-medium">{template.name}</td>
								<td class="text-surface-300 px-4 py-4">
									<div class="flex items-center gap-2">
										<span class="truncate"
											>http://{$settings.server.host}:{$settings.server
												.port}/m3u/{template.name}.m3u</span
										>
										<button
											class="text-primary-400 hover:text-primary-300 transition-colors"
											onclick={() => {
												navigator.clipboard.writeText(
													`http://${$settings.server.host}:${$settings.server.port}/m3u/${template.name}.m3u`
												);
												// Could add a toast notification here
											}}
										>
											<Icon icon="mdi:content-copy" width="16" height="16" />
										</button>
									</div>
								</td>
								<td class="text-surface-300 px-4 py-4">
									<div class="flex items-center gap-2">
										<span class="truncate"
											>http://{$settings.server.host}:{$settings.server
												.port}/xmltv/{template.name}.xml</span
										>
										<button
											class="text-primary-400 hover:text-primary-300 transition-colors"
											onclick={() => {
												navigator.clipboard.writeText(
													`http://${$settings.server.host}:${$settings.server.port}/xmltv/${template.name}.xml`
												);
												// Could add a toast notification here
											}}
										>
											<Icon icon="mdi:content-copy" width="16" height="16" />
										</button>
									</div>
								</td>
								<td class="px-4 py-4 text-center">
									<button
										class="btn bg-primary-700 hover:bg-primary-600 rounded-lg p-2 text-white transition-colors"
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
												<FloatingArrow
													bind:ref={elemArrow}
													context={refreshTooltipFloating.context}
													fill="#1e293b"
												/>
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
			<div class="flex flex-col items-center justify-center px-4 py-12 text-center">
				<Icon icon="mdi:playlist-remove" class="text-surface-500 mb-4" width="48" height="48" />
				<h3 class="text-surface-300 mb-2 text-xl font-medium">No Templates Found</h3>
				<p class="text-surface-400 max-w-md">
					Create your first template to get started with Xivi.
				</p>
			</div>
		{/if}
	</div>
</section>

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	contentBase="card bg-surface-800 p-0 shadow-xl max-w-screen-sm border border-surface-700/50 rounded-lg overflow-hidden"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
		<div class="w-modal">
			<header class="bg-surface-700/30 border-surface-700/30 border-b p-4">
				<h3 class="text-primary-300 flex items-center gap-2 text-xl font-bold">
					<Icon icon="mdi:information-outline" width="24" height="24" />
					<span>Notification</span>
				</h3>
			</header>

			<div class="p-6">
				<p class="text-surface-200">{modalContent}</p>
			</div>

			<footer class="bg-surface-700/20 border-surface-700/30 flex justify-end border-t p-4">
				<button
					class="btn bg-primary-700 hover:bg-primary-600 flex items-center gap-2 rounded-lg px-4 py-2 text-white transition-colors duration-200"
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
