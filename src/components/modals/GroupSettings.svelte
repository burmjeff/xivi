<!-- GroupSettings.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';

	// Note: Svelte components don't need default exports
	// They are automatically exported
	import {
		Switch,
		Combobox } from '@skeletonlabs/skeleton-svelte';
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
		useRole,
	} from "@skeletonlabs/floating-ui-svelte";
	import { fade } from "svelte/transition";

	const { parent, isNew, name, dynamic, dynamicgroup } = $props();

	// Initialize with empty string array for the Combobox, similar to ChannelSettings.svelte
	let selectedOption = $state([""]);

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
			dynamicgroup: dynamicgroup,
		};
	} else {
		formData = {
			name: "",
			dynamic: false,
			dynamicgroup: 0,
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
		placement: "top",
		get middleware() {
			return [offset(10), flip(), elemArrow && arrow({ element: elemArrow })];
		},
	});

	// Interactions
	const tooltipRole = useRole(tooltipFloating.context, { role: "tooltip" });
	const tooltipHover = useHover(tooltipFloating.context, { move: false });
	const tooltipDismiss = useDismiss(tooltipFloating.context);
	const tooltipInteractions = useInteractions([tooltipRole, tooltipHover, tooltipDismiss]);

	onMount(async () => {
		console.log('GroupSettings onMount called with dynamic:', dynamic, 'dynamicgroup:', dynamicgroup);
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
							label: `${$playlists[i].name} - ${group.name}`,// (ID: ${group.id})`,
							value: group.id,
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

		// The issue is that dynamicOptions is empty at this point, even though we've added items to it
		// Let's use setTimeout to ensure all reactivity updates have been processed
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
			if (selectedOption.length == 0 || selectedOption[0] === "") {
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

<div class="modal-add-group">
	{#if isNew}
		<header class="justify-center text-center text-2xl font-bold">Add Group</header>
	{:else}
		<header class="justify-center text-center text-2xl font-bold">Modify Group</header>
	{/if}
	<div class="">
		<label class="template_group_name p-2">
			<span>Template Group Name</span>
			<input
				class="input"
				type="text"
				bind:value={formData.name}
				placeholder="Template Group Name"
			/>
		</label>
		<div class="grid grid-cols-2 items-center gap-4 p-2">
			<div class="template_group_dynamic flex items-center justify-between" bind:this={tooltipFloating.elements.reference}>
				<span>Dynamic Group</span>
				<Switch
					name="dynamic"
					base="flex"
					controlBase="relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
					controlActive="bg-primary-500"
					controlInactive="preset-filled-surface-200-800"
					thumbBase="pointer-events-none block h-5 w-5 rounded-full bg-white shadow-lg ring-0 transition-transform"
					thumbActive="translate-x-5"
					thumbInactive="translate-x-1"
					checked={formData.dynamic}
					onCheckedChange={(e) => (formData.dynamic = e.checked)}
					{...tooltipInteractions.getReferenceProps()}
				/>
			</div>
			<div class="template_group_playlist flex flex-col">
				{#if formData.dynamic}
					<span>Select Playlist Group</span>
					{#if dynamicOptions.length > 0}
						<!-- Add defaultValue to ensure initial selection is shown -->
						<Combobox
							data={dynamicOptions}
							value={selectedOption}
							defaultValue={selectedOption}
							onValueChange={(e) => (selectedOption = e.value)}
							label=""
							placeholder="Select or type..."
							positioning={{
								placement: 'bottom-start',
								flip: false,
								overflowPadding: 8,
								fitViewport: true
							}}
							contentBase="max-h-48 overflow-y-auto"
						>
						<!-- This is optional. Combobox will render label by default -->
						{#snippet item(item: {label: string; value: string})}
							<div class="flex w-full justify-between space-x-2">
								<span>{item.label}</span>
							</div>
						{/snippet}
					</Combobox>
				{:else}
					<div class="input p-2 text-gray-500">No playlist groups available</div>
				{/if}
			{/if}
			</div>
		</div>
		{#if tooltipOpen}
			<div
				bind:this={tooltipFloating.elements.floating}
				style={tooltipFloating.floatingStyles}
				{...tooltipInteractions.getFloatingProps()}
				class="floating popover-neutral"
				transition:fade={{ duration: 200 }}
			>
				<p>
					<strong>Enable Dynamic Group</strong>
				</p>
				<FloatingArrow bind:ref={elemArrow} context={tooltipFloating.context} fill="#575969" />
			</div>
		{/if}
		<footer class="modal-footer flex justify-end gap-4">
			<button class="btn preset-outlined-surface-500" onclick={onCancel}>Cancel</button>
			<button class="btn preset-filled-primary-500" onclick={onFormSubmit}>Save</button>
		</footer>
	</div>
</div>
