<!-- AddGroup.svelte -->

<script lang="ts">
	import { onMount, type SvelteComponent } from 'svelte';
	import {
		popup,
		getModalStore,
		SlideToggle,
		InputChip,
		Autocomplete,
		type AutocompleteOption,
		type PopupSettings
	} from '@skeletonlabs/skeleton';
	import { playlists } from '@xivi/stores/playlist_store';
	import type { PlaylistGroup } from '@xivi/data/playlist_entities';

	//export let parent: SvelteComponent;

	const modalStore = getModalStore();
	let isNew = $modalStore[0].meta.isNew;
	let dynamicNames: string[];
	let addedLabels: string[];
	let dynamicOptions: AutocompleteOption<string>[];
	let inputPlaylist: string;

	let formData: {
		name: string;
		dynamic: boolean;
		dynamicgroup: number;
	};

	interface playlistItem {
		id: number;
		playlist_id: number;
		group_id: number;
	}
	const playlistItems = [] as Array<playlistItem>;

	if (!isNew) {
		formData = {
			name: $modalStore[0].meta.name,
			dynamic: $modalStore[0].meta.dynamic,
			dynamicgroup: $modalStore[0].meta.dynamicgroup
		};
	} else {
		formData = {
			name: '',
			dynamic: false,
			dynamicgroup: 0
		};
	}

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

	const getPlaylistGroupItems = async () => {
		const response = await fetch(`/api/playlist/group/items`);
		const data = await response.json();
		return data.items;
	};

	let popupDynamic: PopupSettings = {
		event: 'focus-click',
		target: 'popupAutocomplete',
		placement: 'right'
	};

	onMount(async () => {
		const fetchedItems = await getPlaylistGroupItems();
		if (typeof fetchedItems !== 'undefined') {
			fetchedItems.forEach(function (item: playlistItem) {
				playlistItems.push(item);
			});
		}

		if ($playlists.length == 0) {
			playlists.set(await updatePlaylists());
		}
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

			dynamicNames = $playlists[i].groups.map((group) => {
				return `${$playlists[i].name} - ${group.name}`;
			});
			dynamicOptions = $playlists[i].groups.map((group) => {
				return {
					label: `${$playlists[i].name} - ${group.name}`,
					value: `${$playlists[i].name} - ${group.name}`,
					meta: `${$playlists[i].id},${group.id}`
				};
			});
		}

		if (!isNew && formData.dynamicgroup !== 0 && formData.dynamicgroup !== undefined) {
			let item =
				playlistItems[playlistItems.findIndex((item) => item.id === formData.dynamicgroup)];
			let playlistIdx = $playlists.findIndex((playlist) => playlist.id === item.playlist_id);
			let groupIdx = $playlists[playlistIdx].groups.findIndex(
				(group) => Number(group.id) === item.group_id
			);
			addedLabels[0] = `${$playlists[playlistIdx].name} - ${$playlists[playlistIdx].groups[groupIdx].name}`;
		}
	});

	function onInputChipSelect(event: CustomEvent<AutocompleteOption<string>>): void {
		if (addedLabels.length === 0) {
			addedLabels.push(event.detail.label);
			addedLabels = [...addedLabels];
		}
	}

	async function onFormSubmit(): Promise<void> {
		if (formData.dynamic) {
			if (addedLabels.length == 0) {
				formData.dynamic = false;
				formData.dynamicgroup = 0;
			} else {
				const dynamicItem = (<string>(
					dynamicOptions[dynamicOptions.findIndex((item) => item.label === addedLabels[0])].meta
				)).split(',');
				formData.dynamicgroup =
					playlistItems[
						playlistItems.findIndex(
							(item) =>
								item.playlist_id === Number(dynamicItem[0]) &&
								item.group_id === Number(dynamicItem[1])
						)
					].id;
			}
		} else {
			formData.dynamicgroup = 0;
		}
		if ($modalStore[0].response) $modalStore[0].response(formData);
		modalStore.close();
	}
</script>

{#if $modalStore[0]}
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
				<SlideToggle
					class="h-fit w-fit"
					name="slide"
					active="bg-primary-500"
					bind:checked={formData.dynamic}>Dynamic</SlideToggle
				>
				<div class="template_group_playlist" use:popup={popupDynamic}>
					<span>Playlist Group</span>
					<InputChip
						bind:input={inputPlaylist}
						bind:value={addedLabels}
						whitelist={dynamicNames}
						max={1}
						placeholder="Search..."
						name="chips"
					/>
					<div data-popup="popupAutocomplete" class="card max-h-96 w-fit overflow-y-scroll">
						<Autocomplete
							bind:input={inputPlaylist}
							options={dynamicOptions}
							allowlist={dynamicNames}
							on:selection={onInputChipSelect}
						/>
					</div>
				</div>
			</div>
			<div class="submit_button justify-center p-2 text-center">
				<button class="btn h-fit w-fit bg-primary-500" on:click={onFormSubmit}
					>Save Template Group</button
				>
			</div>
		</div>
	</div>
{/if}
