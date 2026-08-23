<!-- GroupSettings.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import { Switch, Combobox } from '@skeletonlabs/skeleton-svelte';
	import { playlists } from '@xivi/stores/playlist_store';
	import type { PlaylistGroup } from '@xivi/data/playlist_entities';
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

	const { parent, isNew, name, dynamic, dynamicgroup } = $props();

	// Initialize with empty string array for the Combobox, similar to ChannelSettings.svelte
	let selectedOption = $state(['']);

	// Log initial values in onMount to avoid state reference issues

	interface DynamicOptions {
		label: string;
		value: string;
	}
	let dynamicOptions: DynamicOptions[] = $state([]);

	// State for floating UI
	let tooltipOpen = $state(false);
	let elemArrow: HTMLElement | null = $state(null);

	let formData: {
		name: string;
		dynamic: boolean;
		dynamicgroup: number;
	} = $state({
		name,
		dynamic,
		dynamicgroup
	});

	if (!isNew) {
		formData = {
			name: name,
			dynamic: dynamic,
			dynamicgroup: dynamicgroup
		};
	} else {
		formData = {
			name: '',
			dynamic: false,
			dynamicgroup: 0
		};
	}

	const updatePlaylists = async () => {
		try {
			console.log('Fetching playlists');
			const response = await fetch('/api/playlists');
			if (!response.ok) {
				console.error(`Error fetching playlists: ${response.status} ${response.statusText}`);
				return [];
			}
			const data = await response.json();
			console.log('Playlists data:', data);
			return data.playlists || [];
		} catch (error) {
			console.error('Error in updatePlaylists:', error);
			return [];
		}
	};

	const updatePlaylistGroups = async (playlistId: number) => {
		try {
			console.log(`Fetching groups for playlist ID: ${playlistId}`);
			const response = await fetch(`/api/playlist/${playlistId}/groups`);
			if (!response.ok) {
				console.error(`Error fetching playlist groups: ${response.status} ${response.statusText}`);
				return [];
			}
			const data = await response.json();
			console.log('Playlist groups data:', data);
			return data.playlistgroups || [];
		} catch (error) {
			console.error('Error in updatePlaylistGroups:', error);
			return [];
		}
	};

	// Floating UI setup
	const tooltipFloating = useFloating({
		whileElementsMounted: autoUpdate,
		get open() {
			return tooltipOpen;
		},
		onOpenChange: (v) => {
			tooltipOpen = v;
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

	onMount(async () => {
		console.log(
			'GroupSettings onMount called with dynamic:',
			dynamic,
			'dynamicgroup:',
			dynamicgroup
		);
		if ($playlists.length == 0) {
			playlists.set(await updatePlaylists());
		}
		dynamicOptions = [];

		for (let i = 0; i < $playlists.length; i++) {
			// Make sure playlist exists
			if (!$playlists[i]) {
				console.error(`Playlist at index ${i} is undefined`);
				continue;
			}

			// Initialize groups array if it doesn't exist
			if ($playlists[i].groups == undefined) {
				$playlists[i].groups = [];
				try {
					const fetchedData = await updatePlaylistGroups($playlists[i].id);
					console.log('Fetched playlist groups:', fetchedData);
					if (Array.isArray(fetchedData) && fetchedData.length > 0) {
						// Make sure groups array is initialized
						if (!Array.isArray($playlists[i].groups)) {
							$playlists[i].groups = [];
						}

						// Add each group to the array
						const newGroups = [...$playlists[i].groups]; // Create a new array to avoid direct mutation
						fetchedData.forEach(function (group: PlaylistGroup) {
							newGroups.push(group);
						});

						// Update the reference to trigger reactivity
						$playlists[i].groups = newGroups;
						console.log(`Updated playlist ${$playlists[i].name} with ${fetchedData.length} groups`);
					}
				} catch (error) {
					console.error('Error fetching playlist groups:', error);
				}
			}

			// Make sure groups array exists and has items before trying to use it
			if (Array.isArray($playlists[i].groups) && $playlists[i].groups.length > 0) {
				const groupOptions = $playlists[i].groups
					.filter((group) => group && group.enabled)
					.map((group) => {
						// Create a unique label by adding the group ID to ensure uniqueness
						return {
							label: `${$playlists[i].name} - ${group.name}`, // (ID: ${group.id})`,
							value: group.id
						};
					});

				if (groupOptions.length > 0) {
					console.log(`Adding ${groupOptions.length} options from playlist ${$playlists[i].name}`);
					dynamicOptions.push(...groupOptions);
				}
			}
		}

		// Set the initial selected option for the dynamicgroup combobox
		console.log('Setting initial dynamicgroup value:', formData.dynamicgroup);
		console.log('Dynamic options available:', dynamicOptions);

		setTimeout(() => {
			console.log('In setTimeout - Dynamic options available:', dynamicOptions);

			// Make sure we have all options loaded before setting the selected value
			if (formData.dynamic && formData.dynamicgroup !== 0) {
				// Convert the numeric ID to string for comparison
				const dynamicGroupId = formData.dynamicgroup.toString();
				console.log('In setTimeout - Setting selectedOption to new array with:', dynamicGroupId);

				// Create a completely new array to ensure reactivity
				selectedOption = [dynamicGroupId];
				console.log('In setTimeout - Selected option after assignment:', selectedOption);
			}
		}, 100); // Small delay to ensure dynamicOptions is populated

		console.log(`Final dynamicOptions count: ${dynamicOptions.length}`);
	});

	async function onFormSubmit(): Promise<void> {
		console.log('Form submitted with data:', formData);
		console.log('Selected option at form submit:', selectedOption);

		if (formData.dynamic) {
			if (selectedOption.length == 0 || selectedOption[0] === '') {
				console.log('No option selected, disabling dynamic group');
				formData.dynamic = false;
				formData.dynamicgroup = 0;
			} else {
				formData.dynamicgroup = Number(selectedOption[0]);
				console.log('Setting dynamicgroup to:', formData.dynamicgroup);
			}
		} else {
			formData.dynamicgroup = 0;
		}
		console.log('Calling parent.onClose with formData:', formData);
		parent.onClose(formData);
	}

	function onCancel(): void {
		console.log('Cancel button clicked');
		parent.onClose();
	}
</script>

<div class="modal-group">
	{#if isNew}
		<header class="mb26 text-center text-2xl font-bold">Add Group</header>
	{:else}
		<header class="mb-2 text-center text-2xl font-bold">Modify Group</header>
	{/if}
	<div class="space-y-4">
		<div class="form-group">
			<label class="mb-2 block text-sm font-medium" for="group_name"> Template Group Name </label>
			<input
				id="group_name"
				class="input w-full"
				type="text"
				bind:value={formData.name}
				placeholder="Enter group name"
				required
			/>
		</div>

		<div class="form-section">
			<div class="form-row">
				<div class="form-group-inline" bind:this={tooltipFloating.elements.reference}>
					<span class="text-sm font-medium">Dynamic Group</span>
					<Switch
						name="dynamic"
						checked={formData.dynamic}
						onCheckedChange={(e) => (formData.dynamic = e.checked)}
						{...tooltipInteractions.getReferenceProps()}
					>
						<Switch.Control
							class="bg-surface-300 data-[state=checked]:bg-primary-500 relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
						>
							<Switch.Thumb
								class="pointer-events-none block h-5 w-5 translate-x-1 rounded-full bg-white shadow-lg ring-0 transition-transform data-[state=checked]:translate-x-5"
							/>
						</Switch.Control>
					</Switch>
				</div>

				<div class="form-group flex-1">
					{#if formData.dynamic}
						<div class="combobox-wrapper">
							{#if dynamicOptions.length > 0}
								<Combobox value={selectedOption} onValueChange={(e) => (selectedOption = e.value)}>
									<Combobox.Label class="mb-2 block text-sm font-medium"
										>Select Playlist Group</Combobox.Label
									>
									<div class="relative">
										<Combobox.Input class="input w-full" placeholder="Select or type..." />
									</div>
									<Combobox.Content
										class="glass card z-[9999] max-h-48 overflow-y-auto p-2 shadow-lg"
									>
										{#each dynamicOptions as option}
											<Combobox.Item
												value={option.value as any}
												label={option.label}
												class="hover:bg-surface-700/30 cursor-pointer rounded px-2 py-1"
											>
												{option.label}
											</Combobox.Item>
										{/each}
									</Combobox.Content>
								</Combobox>
							{:else}
								<div class="input bg-surface-700/30 p-2 text-gray-500">
									No playlist groups available
								</div>
							{/if}
						</div>
					{/if}
				</div>
			</div>
		</div>

		{#if tooltipOpen}
			<div
				bind:this={tooltipFloating.elements.floating}
				style={tooltipFloating.floatingStyles}
				{...tooltipInteractions.getFloatingProps()}
				class="floating glass card z-50 p-2 shadow-lg"
				transition:fade={{ duration: 200 }}
			>
				<p class="text-sm font-medium">
					<strong>Enable Dynamic Group</strong>
				</p>
				<FloatingArrow bind:ref={elemArrow} context={tooltipFloating.context} fill="#1e293b" />
			</div>
		{/if}

		<footer class="modal-footer border-surface-600 flex justify-end gap-4 border-t pt-4">
			<button class="btn preset-outlined-surface-500 min-w-20" onclick={onCancel}> Cancel </button>
			<button class="btn preset-filled-primary-500 min-w-20" onclick={onFormSubmit}> Save </button>
		</footer>
	</div>
</div>

<style>
	.modal-group {
		min-width: 500px;
		padding: 0.2rem 0.5rem;
	}

	.form-group {
		display: flex;
		flex-direction: column;
	}

	.form-group label {
		color: var(--color-surface-200);
		font-weight: 500;
	}

	.form-group input {
		transition: all 0.2s ease;
	}

	.form-group input:focus {
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	}

	.form-section {
		background: var(--color-surface-800/30);
		border: 1px solid var(--color-surface-600/50);
		border-radius: 0.5rem;
		padding: 1rem;
	}

	.form-row {
		display: flex;
		gap: 1.5rem;
		align-items: flex-start;
	}

	.form-group-inline {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.5rem;
		min-width: 120px;
	}

	.form-group-inline span {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-surface-200);
		text-align: center;
	}

	.modal-footer {
		margin-top: 2rem;
	}

	.modal-footer button {
		font-weight: 500;
		padding: 0.4rem 2rem;
	}

	/* Mobile responsive */
	@media (max-width: 768px) {
		.modal-group {
			min-width: unset;
			padding: 1rem;
		}

		.form-row {
			flex-direction: column;
			gap: 1rem;
		}

		.form-group-inline {
			flex-direction: row;
			justify-content: space-between;
			min-width: unset;
			width: 100%;
		}

		.modal-footer {
			flex-direction: column;
			gap: 0.75rem;
		}

		.modal-footer button {
			width: 100%;
		}
	}
</style>
