<!-- GroupSettings.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Switch,
		Combobox,
		Modal } from '@skeletonlabs/skeleton-svelte';
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

	let { modalOpen = $bindable(false), parent, isNew, name, dynamic, dynamicgroup } = $props<{
		modalOpen: boolean;
		parent: any;
		isNew: boolean;
		name: string;
		dynamic: boolean;
		dynamicgroup: number;
	}>();

	let selectedOption = $state([""]);

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

	const updatePlaylists = async () => {
		const response = await fetch('/api/playlists');
		const data = await response.json();
		return data.playlists;
	};

	const updatePlaylistGroups = async (playlistId: number) => {
		const response = await fetch(`/api/playlist/${playlistId}/groups`);
		const data = await response.json();
		return data.playlistgroups;
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
		if ($playlists.length == 0) {
			playlists.set(await updatePlaylists());
		}
		dynamicOptions = [];

		for (let i = 0; i < $playlists.length; i++) {
			if ($playlists[i].groups == undefined) {
				$playlists[i].groups = [];
				const fetchedData = await updatePlaylistGroups($playlists[i].id);
				if (typeof fetchedData !== 'undefined') {
					fetchedData.forEach(function (group: PlaylistGroup) {
						$playlists[i].groups.push(group);
						$playlists[i].groups = $playlists[i].groups;
					});
				}
			}

			dynamicOptions.push(...$playlists[i].groups
				.filter((group) => group.enabled)
				.map((group) => {
					return {
						label: `${$playlists[i].name} - ${group.name}`,
						value: group.id
					};
				})
			);
		}
	});

	async function onFormSubmit(): Promise<void> {
		if (formData.dynamic) {
			if (selectedOption.length == 0) {
				formData.dynamic = false;
				formData.dynamicgroup = 0;
			} else {
				formData.dynamicgroup = Number(selectedOption[0]);
			}
		} else {
			formData.dynamicgroup = 0;
		}
		parent.onClose(formData);
		modalClose();
	}

	function onCancel(): void {
        parent.onClose();
    }

	function modalClose() {
    modalOpen = false;
  }
</script>

<Modal
	open={modalOpen}
	onOpenChange={(e) => (modalOpen = e.open)}
	contentBase="card bg-surface-100-900 p-4 space-y-4 shadow-xl max-w-screen-sm"
	backdropClasses="backdrop-blur-sm"
>
	{#snippet content()}
		<div class="modal-add-group max-w-screen card max-h-screen w-fit space-y-4 p-4 shadow-xl">
			{#if isNew}
				<header class="justify-center text-center text-2xl font-bold">Add Group</header>
			{:else}
				<header class="justify-center text-center text-2xl font-bold">Modify Group</header>
			{/if}
			<div class="space-y-4 divide-y divide-dashed divide-teal-400">
				<label class="template_group_name p-2">
					<span>Template Group Name</span>
					<input
						class="input"
						type="text"
						bind:value={formData.name}
						placeholder="Template Group Name"
					/>
				</label>
				<div class="grid h-fit w-fit grid-cols-2 items-center gap-4 p-2">
					<div class="template_group_dynamic h-fit w-fit" bind:this={tooltipFloating.elements.reference}>
						<Switch
							name="slide"
							checked={formData.dynamic}
							onCheckedChange={(e) => (formData.dynamic = e.checked)}

							{...tooltipInteractions.getReferenceProps()}
						/>
					</div>
					<div class="template_group_playlist">
						<span>Playlist Group</span>
						<Combobox
							data={dynamicOptions}
							value={selectedOption}
							onValueChange={(e) => (selectedOption = e.value)}
							label="Select Group"
							placeholder="Select..."
							>
							<!-- This is optional. Combobox will render label by default -->
							{#snippet item(item)}
								<div class="flex w-full justify-between space-x-2">
								<span>{item.label}</span>
								</div>
							{/snippet}
						</Combobox>
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
				<footer class="modal-footer {parent.regionFooter}">
					<button class="btn variant-ghost-surface" onclick={onCancel}>Cancel</button>
					<button class="btn variant-filled-primary" onclick={onFormSubmit}>Save</button>
				</footer>
			</div>
		</div>
	{/snippet}
</Modal>
