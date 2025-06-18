<!-- Epg.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import { epgs } from '@xivi/stores/epg_store';
	import Icon from '@iconify/svelte';
	import { Modal } from '@skeletonlabs/skeleton-svelte';
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
		useRole
	} from '@skeletonlabs/floating-ui-svelte';
	import { fade } from 'svelte/transition';

	const updateEpgs = async () => {
		const response = await fetch('/api/epgs');
		const data = await response.json();
		return data.epgs;
	};

	onMount(async () => {
		epgs.set(await updateEpgs());
	});

	// State
	let tooltipopen = $state(false);
	let addOpen = $state(false);
	let elemArrow: HTMLElement | null = $state(null);
	let editModalOpen = $state(false);
	let deleteModalOpen = $state(false);
	let editNameTooltipOpen = $state(false);
	let editUrlTooltipOpen = $state(false);
	let deleteTooltipOpen = $state(false);
	let refreshTooltipOpen = $state(false);
	let refreshingEpgId = $state<number | null>(null);

	let currentEpgIdx: number;
	let editType: string = $state('');
	let editValue: string = $state('');
	let epgToDelete: number | null = null;

	// Format date to YYYY-MM-DD HH:MM
	function formatDateTime(dateStr: string) {
		const date = new Date(dateStr);
		let year = date.getFullYear();
		let month = String(date.getMonth() + 1).padStart(2, '0');
		let day = String(date.getDate()).padStart(2, '0');
		let hours = String(date.getHours()).padStart(2, '0');
		let minutes = String(date.getMinutes()).padStart(2, '0');
		return `${year}-${month}-${day} ${hours}:${minutes}`;
	}

	// Use Floating
	const tooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return tooltipopen;
		},
		onOpenChange: (v) => {
			tooltipopen = v;
		},
		placement: 'top',
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		}
	});
	const addFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return addOpen;
		},
		onOpenChange: (v) => {
			addOpen = v;
		},
		placement: 'top',
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		}
	});

	// Interactions
	const tooltipRole = useRole(tooltipFloating.context, { role: 'tooltip' });
	const tooltipHover = useHover(tooltipFloating.context, { move: false });
	const tooltipDismiss = useDismiss(tooltipFloating.context);
	const tooltipInteractions = useInteractions([tooltipRole, tooltipHover, tooltipDismiss]);

	const addRole = useRole(addFloating.context);
	const addClick = useClick(addFloating.context);
	const addDismiss = useDismiss(addFloating.context);
	const addInteractions = useInteractions([addRole, addClick, addDismiss]);

	// Floating UI setup for edit name tooltip
	const editNameTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return editNameTooltipOpen;
		},
		onOpenChange: (v) => {
			editNameTooltipOpen = v;
		},
		placement: 'top',
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		}
	});

	// Interactions for edit name tooltip
	const editNameTooltipRole = useRole(editNameTooltipFloating.context, { role: 'tooltip' });
	const editNameTooltipHover = useHover(editNameTooltipFloating.context, { move: false });
	const editNameTooltipDismiss = useDismiss(editNameTooltipFloating.context);
	const editNameTooltipInteractions = useInteractions([
		editNameTooltipRole,
		editNameTooltipHover,
		editNameTooltipDismiss
	]);

	// Floating UI setup for edit URL tooltip
	const editUrlTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return editUrlTooltipOpen;
		},
		onOpenChange: (v) => {
			editUrlTooltipOpen = v;
		},
		placement: 'top',
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		}
	});

	// Interactions for edit URL tooltip
	const editUrlTooltipRole = useRole(editUrlTooltipFloating.context, { role: 'tooltip' });
	const editUrlTooltipHover = useHover(editUrlTooltipFloating.context, { move: false });
	const editUrlTooltipDismiss = useDismiss(editUrlTooltipFloating.context);
	const editUrlTooltipInteractions = useInteractions([
		editUrlTooltipRole,
		editUrlTooltipHover,
		editUrlTooltipDismiss
	]);

	// Floating UI setup for delete tooltip
	const deleteTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return deleteTooltipOpen;
		},
		onOpenChange: (v) => {
			deleteTooltipOpen = v;
		},
		placement: 'top',
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		}
	});

	// Interactions for delete tooltip
	const deleteTooltipRole = useRole(deleteTooltipFloating.context, { role: 'tooltip' });
	const deleteTooltipHover = useHover(deleteTooltipFloating.context, { move: false });
	const deleteTooltipDismiss = useDismiss(deleteTooltipFloating.context);
	const deleteTooltipInteractions = useInteractions([
		deleteTooltipRole,
		deleteTooltipHover,
		deleteTooltipDismiss
	]);

	// Floating UI setup for refresh tooltip
	const refreshTooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return refreshTooltipOpen;
		},
		onOpenChange: (v) => {
			refreshTooltipOpen = v;
		},
		placement: 'top',
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

	function handleConfirm() {
		if (editValue) {
			editEpg(currentEpgIdx, editType, editValue);
		}
		editModalClose();
	}

	function editModalClose() {
		editModalOpen = false;
	}

	async function addEpg() {
		const inputName = (document.querySelector('.epg_name input') as HTMLInputElement).value;
		const inputUrl = (document.querySelector('.epg_url input') as HTMLInputElement).value;
		if (inputName !== '' && inputUrl !== '') {
			const newEpg = {
				name: inputName,
				url: inputUrl
			};
			window.console.log('EPG: ', newEpg);

			try {
				const response = await fetch('/api/epg', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(newEpg)
				});
				const data = await response.json();
				console.log('Created epg:', data);
				$epgs.push(data.epg);
				$epgs = [...$epgs];
			} catch (error) {
				console.log('Error creating epg:', error);
			}
		}
	}

	function editPrompt(epgIdx: number, type: string): void {
		let initialValue = type === 'name' ? $epgs[epgIdx].name : $epgs[epgIdx].url;
		editModalOpen = true;
		currentEpgIdx = epgIdx;
		editType = type;
		editValue = initialValue;
	}

	function deletePrompt(epgId: number): void {
		epgToDelete = epgId;
		deleteModalOpen = true;
	}

	function handleDelete() {
		if (epgToDelete !== null) {
			deleteEpg(epgToDelete);
		}
		deleteModalOpen = false;
	}

	async function editEpg(epgIdx: number, type: string, newValue: string) {
		if (type == 'name') {
			$epgs[epgIdx].name = newValue;
		} else if (type == 'url') {
			$epgs[epgIdx].url = newValue;
		}

		try {
			const response = await fetch('/api/epg', {
				method: 'PUT',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify($epgs[epgIdx])
			});

			if (response.ok) {
				console.log('Updated epg: ', newValue);
				$epgs = [...$epgs];
			} else {
				console.error('Error:', response.status, response.statusText);
			}
		} catch (error) {
			console.log('Error updating epg:', error);
		}
	}

	async function deleteEpg(epgId: number) {
		try {
			const response = await fetch(`/api/epg/${epgId}`, {
				method: 'DELETE'
			});
			const data = response.status;
			console.log('Deleted epg:', data);
			$epgs = $epgs.filter((t) => t.id != epgId);
		} catch (error) {
			console.log('Error deleting epg:', error);
			return;
		}
	}

	async function refreshEpg(epgId: number) {
		try {
			// Set the refreshing state for this EPG
			refreshingEpgId = epgId;

			// Call the API to refresh the EPG
			const response = await fetch(`/api/epg/${epgId}/refresh`, {
				method: 'POST'
			});

			if (response.ok) {
				console.log('Refreshing EPG started');

				// Wait a bit to allow the backend to process, then update the UI
				setTimeout(async () => {
					// Fetch updated EPGs
					epgs.set(await updateEpgs());
					// Clear the refreshing state
					refreshingEpgId = null;
				}, 2000);
			} else {
				console.error('Error refreshing EPG:', response.status, response.statusText);
				refreshingEpgId = null;
			}
		} catch (error) {
			console.log('Error refreshing EPG:', error);
			refreshingEpgId = null;
		}
	}
</script>

<section class="epgs h-full w-full p-1">
	<header class="epgs-header border-surface-700/30 flex items-center justify-center border-b p-1">
		<h4 class="h4 text-primary-400 font-bold">Epgs</h4>
		<button
			class="btn btn-md self-start"
			bind:this={tooltipFloating.elements.reference}
			bind:this={addFloating.elements.reference}
			{...tooltipInteractions.getReferenceProps()}
			{...addInteractions.getReferenceProps()}
		>
			<Icon icon="icon-park-twotone:add-one" color="#0a7e85" width="25" height="25" />
		</button>
		<!-- Floating Element -->
		{#if tooltipopen}
			<div
				bind:this={tooltipFloating.elements.floating}
				style={tooltipFloating.floatingStyles}
				{...tooltipInteractions.getFloatingProps()}
				class="floating glass card p-2 shadow-lg"
				transition:fade={{ duration: 200 }}
			>
				<p class="text-sm font-medium">
					<strong>Add New EPG</strong>
				</p>
				<FloatingArrow bind:ref={elemArrow} context={tooltipFloating.context} fill="#1e293b" />
			</div>
		{/if}
		{#if addOpen}
			<div
				bind:this={addFloating.elements.floating}
				style={addFloating.floatingStyles}
				{...addInteractions.getFloatingProps()}
				class="floating popover-neutral"
				transition:fade={{ duration: 200 }}
			>
				<div class="card gap-4 p-4">
					<h2>Add Epg</h2>
					<div class="space-y-4">
						<label class="epg_name">
							<span>Epg Name</span>
							<input class="input" type="text" placeholder="Epg Name" />
						</label>
						<label class="epg_url">
							<span>Epg url</span>
							<input class="input" type="url" placeholder="https://example.com/xivi.xmltv" />
						</label>
						<label class="submit_button">
							<button class="btn bg-primary-500" onclick={addEpg}>Add Epg</button>
						</label>
					</div>
				</div>
				<FloatingArrow bind:ref={elemArrow} context={addFloating.context} fill="#1e293b" />
			</div>
		{/if}
	</header>
	<div id="accord" class="epgs-viewport min-w-full overflow-auto">
		{#if $epgs != null}
			<table class="epgTable table">
				<thead>
					<tr>
						<th class="text-center">Name</th>
						<th class="text-center">Url</th>
						<th class="text-center">Updated</th>
						<th class="text-center">Refresh</th>
						<th class="text-center">Delete</th>
					</tr>
				</thead>
				<tbody class="items-center">
					{#if $epgs.length > 0}
						{#each $epgs as epg, epgIdx (epg.id)}
							<tr id="animate">
								<td>
									<div class="flex items-center justify-center gap-2">
										<span>{epg.name}</span>
										<button
											class="btn-icon btn-icon-sm inset-y-0 bg-transparent!"
											onclick={() => {
												editPrompt(epgIdx, 'name');
											}}
											bind:this={editNameTooltipFloating.elements.reference}
											{...editNameTooltipInteractions.getReferenceProps()}
										>
											<Icon icon="icon-park-outline:edit-one" width="18" height="18" />
											{#if editNameTooltipOpen}
												<div
													bind:this={editNameTooltipFloating.elements.floating}
													style={editNameTooltipFloating.floatingStyles}
													{...editNameTooltipInteractions.getFloatingProps()}
													class="floating glass card p-2 shadow-lg"
													transition:fade={{ duration: 200 }}
												>
													<p class="text-sm font-medium"><strong>Edit EPG Name</strong></p>
													<FloatingArrow
														bind:ref={elemArrow}
														context={editNameTooltipFloating.context}
														fill="#1e293b"
													/>
												</div>
											{/if}
										</button>
									</div>
								</td>
								<td>
									<div class="flex items-center justify-center gap-2">
										<span>{epg.url}</span>
										<button
											class="btn-icon btn-icon-sm inset-y-0 bg-transparent!"
											onclick={() => {
												editPrompt(epgIdx, 'url');
											}}
											bind:this={editUrlTooltipFloating.elements.reference}
											{...editUrlTooltipInteractions.getReferenceProps()}
										>
											<Icon icon="icon-park-outline:edit-one" width="18" height="18" />
											{#if editUrlTooltipOpen}
												<div
													bind:this={editUrlTooltipFloating.elements.floating}
													style={editUrlTooltipFloating.floatingStyles}
													{...editUrlTooltipInteractions.getFloatingProps()}
													class="floating glass card p-2 shadow-lg"
													transition:fade={{ duration: 200 }}
												>
													<p class="text-sm font-medium"><strong>Edit EPG URL</strong></p>
													<FloatingArrow
														bind:ref={elemArrow}
														context={editUrlTooltipFloating.context}
														fill="#1e293b"
													/>
												</div>
											{/if}
										</button>
									</div>
								</td>
								<td>
									{formatDateTime(epg.updated_at)}
								</td>
								<td class="items-center justify-center">
									<button
										class="btn-icon btn-icon-sm inset-y-0 bg-transparent!"
										onclick={() => {
											refreshEpg(epg.id);
										}}
										bind:this={refreshTooltipFloating.elements.reference}
										{...refreshTooltipInteractions.getReferenceProps()}
										disabled={refreshingEpgId === epg.id}
										class:animate-spin={refreshingEpgId === epg.id}
									>
										<Icon icon="icon-park-outline:refresh-one" width="18" height="18" />
										{#if refreshTooltipOpen}
											<div
												bind:this={refreshTooltipFloating.elements.floating}
												style={refreshTooltipFloating.floatingStyles}
												{...refreshTooltipInteractions.getFloatingProps()}
												class="floating glass card p-2 shadow-lg"
												transition:fade={{ duration: 200 }}
											>
												<p class="text-sm font-medium"><strong>Refresh EPG</strong></p>
												<FloatingArrow
													bind:ref={elemArrow}
													context={refreshTooltipFloating.context}
													fill="#1e293b"
												/>
											</div>
										{/if}
									</button>
								</td>
								<td class="items-center justify-center">
									<button
										class="btn-icon btn-icon-sm inset-y-0 bg-transparent!"
										onclick={() => {
											deletePrompt(epg.id);
										}}
										bind:this={deleteTooltipFloating.elements.reference}
										{...deleteTooltipInteractions.getReferenceProps()}
									>
										<Icon icon="icon-park-outline:delete" width="18" height="18" />
										{#if deleteTooltipOpen}
											<div
												bind:this={deleteTooltipFloating.elements.floating}
												style={deleteTooltipFloating.floatingStyles}
												{...deleteTooltipInteractions.getFloatingProps()}
												class="floating glass card p-2 shadow-lg"
												transition:fade={{ duration: 200 }}
											>
												<p class="text-sm font-medium"><strong>Delete EPG</strong></p>
												<FloatingArrow
													bind:ref={elemArrow}
													context={deleteTooltipFloating.context}
													fill="#1e293b"
												/>
											</div>
										{/if}
									</button>
								</td>
							</tr>
						{/each}
					{:else}
						<tr><td>No epgs found</td></tr>
					{/if}
				</tbody>
			</table>
		{/if}
	</div>
</section>

<Modal
	open={editModalOpen}
	onOpenChange={(e) => (editModalOpen = e.open)}
	triggerBase="btn preset-tonal"
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl max-w-screen-sm"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet trigger()}{/snippet}
	{#snippet content()}
		<header class="flex justify-between">
			<h2 class="h2">Edit EPG {editType}</h2>
		</header>
		<article>
			<label class="label">
				<span>New {editType}</span>
				<input class="input" type="text" bind:value={editValue} placeholder="Enter new value" />
			</label>
		</article>
		<footer class="flex justify-end gap-4">
			<button type="button" class="btn preset-outlined-surface-500" onclick={editModalClose}
				>Cancel</button
			>
			<button type="button" class="btn preset-filled-primary-500" onclick={handleConfirm}
				>Save</button
			>
		</footer>
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
		<header class="flex justify-between">
			<h2 class="h2">Confirm Delete</h2>
		</header>
		<article>
			<p class="opacity-60">Are you sure you wish to delete this EPG?</p>
		</article>
		<footer class="flex justify-end gap-4">
			<button
				type="button"
				class="btn preset-outlined-surface-500"
				onclick={() => (deleteModalOpen = false)}>Cancel</button
			>
			<button type="button" class="btn preset-tonal-error" onclick={handleDelete}>Delete</button>
		</footer>
	{/snippet}
</Modal>

<style>
	#accord {
		max-height: 82vh;
		height: 82vh;
	}

	/* Center all table cells and headers */
	table th,
	table td {
		text-align: center !important;
	}

	#animate {
		position: relative;
	}
</style>
