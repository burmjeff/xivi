<!-- AddGroup.svelte -->

<script lang="ts">
    import { onMount, type SvelteComponent } from 'svelte';
    import { popup, getModalStore, SlideToggle, InputChip, Autocomplete, type AutocompleteOption, type PopupSettings} from '@skeletonlabs/skeleton';
    import {playlistGroups} from '@xivi/stores/playlist_store';

    export let parent: SvelteComponent;

    const modalStore = getModalStore();
    let isNew = $modalStore[0].meta.isNew
    let groupNames: string[];
    let addedNames: string[];
    let groupOptions: AutocompleteOption<string>[];
    let inputPlaylist: string;

    let formData: {
		name: string,
		dynamic: boolean,
		playlistgroup: number
	}

    if (!isNew) {
		formData = {
            name: $modalStore[0].meta.name,
            dynamic: $modalStore[0].meta.dynamic,
            playlistgroup: $modalStore[0].meta.playlistgroup
		};
	} else {
		formData = {
            name: "",
            dynamic: false,
            playlistgroup: 0
        }
	}

    const updatePlaylistGroups = async () => {
        const response = await fetch('/api/playlist/groups/all');
        const data = await response.json();
        return data.playlistgroups;
    }

    let popupDynamic: PopupSettings = {
        event: 'focus-click',
        target: 'popupAutocomplete',
        placement: 'right',
    };

    onMount(async () => {
        playlistGroups.set(await updatePlaylistGroups());
        groupNames = $playlistGroups.map( ( group ) => { return group.name } )
        groupOptions = $playlistGroups.map( ( group ) => { return { label: group.name, value: group.name }} )
        if (!isNew && formData.playlistgroup !== 0) {
            addedNames[0] = $playlistGroups[Number(formData.playlistgroup)].name;
        }
        //groupNames = [...groupNames]
    });

    function onInputChipSelect(event: CustomEvent<AutocompleteOption<string>>): void {
        if (addedNames.length === 0) {
            addedNames.push(event.detail.label);
            addedNames = [...addedNames]
        }
    }

    async function onFormSubmit(): Promise<void> {
        if (formData.dynamic) {
            if (addedNames.length == 0) {
                formData.dynamic = false
                formData.playlistgroup = 0
            } else {
                formData.playlistgroup = Number($playlistGroups[groupNames.findIndex(item => item === addedNames[0])].id);
            }
        } else {
            formData.playlistgroup = 0
        }
		if ($modalStore[0].response) $modalStore[0].response(formData);
		modalStore.close();
	}

</script>

{#if $modalStore[0]}
    <div class="modal-add-group card p-4 w-fit max-w-screen max-h-screen shadow-xl space-y-4">
        {#if isNew}
			<header class="text-2xl font-bold text-center justify-center">Add Group</header>
		{:else}
			<header class="text-2xl font-bold text-center justify-center">Modify Group</header>
		{/if}
        <div class="space-y-4 divide-y divide-dashed divide-teal-400">
            <label class="template_group_name p-2">
                <span>Template Group Name</span>
                <input class="input" type="text" bind:value={formData.name} placeholder="Template Group Name" />
            </label>
            <div class="grid grid-cols-2 gap-4 p-2 items-center w-fit h-fit">
                <SlideToggle class="w-fit h-fit" name="slide" active="bg-primary-500" bind:checked={formData.dynamic}>Dynamic</SlideToggle>
                <div class="template_group_playlist" use:popup={popupDynamic}>
                    <span>Playlist Group</span>
                    <InputChip bind:input={inputPlaylist} bind:value={addedNames} whitelist={groupNames} max={1} placeholder="Search..."  name="chips" />
                    <div data-popup="popupAutocomplete" class="card">
                        <Autocomplete
                            bind:input={inputPlaylist}
                            options={groupOptions}
                            allowlist={groupNames}
                            on:selection={onInputChipSelect}
                        />
                    </div>
                </div>
            </div>
            <div class="submit_button text-center justify-center p-2">
                <button class="btn bg-primary-500 w-fit h-fit" on:click={onFormSubmit}>Save Template Group</button>
            </div>
        </div>
    </div>
{/if}