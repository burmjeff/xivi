<!-- AddGroup.svelte -->

<script lang="ts">
    import { onMount, type SvelteComponent } from 'svelte';
    import { popup, getModalStore, SlideToggle, InputChip, Autocomplete, type AutocompleteOption, type PopupSettings} from '@skeletonlabs/skeleton';
    import {playlistGroups} from '@xivi/stores/playlist_store';
	import type TemplateGroup from '../TemplateGroup.svelte';

    const modalStore = getModalStore();
    let groupNames: string[];
    let addedNames: string[];
    let groupOptions: AutocompleteOption<string>[];
    let inputPlaylist: string;

    let formData = {
		name: "",
		dynamic: false,
		playlistgroup: "0"
	}

    const updatePlaylistGroups = async () => {
        const response = await fetch('/api/playlist/groups/all');
        const data = await response.json();
        return data.playlistgroups;
    }

    let popupDynamic: PopupSettings = {
        event: 'focus-click',
        target: 'popupAutocomplete',
        placement: 'bottom',
    };

    onMount(async () => {
        playlistGroups.set(await updatePlaylistGroups());
        groupNames = $playlistGroups.map( ( group ) => { return group.name } )
        groupOptions = $playlistGroups.map( ( group ) => { return { label: group.name, value: group.name }} )
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
            formData.playlistgroup = $playlistGroups[groupNames.findIndex(item => item === addedNames[0])].id;
        } else {
            formData.playlistgroup = "0"
        }
		if ($modalStore[0].response) $modalStore[0].response(formData);
		modalStore.close();
	}

</script>

{#if $modalStore[0]}
    <div class="modal-add-group card p-4 w-fit shadow-xl space-y-4">
        <h2>Add Template Group</h2>
        <div class="space-y-4 divide-y divide-dashed divide-teal-400">
            <label class="template_group_name p-2">
                <span>Template Group Name</span>
                <input class="input" type="text" bind:value={formData.name} placeholder="Template Group Name" />
            </label>
            <div class="grid grid-cols-2 gap-4 p-6">
                <SlideToggle class="align-middle" name="slide" active="bg-primary-500" bind:checked={formData.dynamic}>Dynamic</SlideToggle>
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
            <label class="submit_button align-middle p-2">
                <button class="btn bg-primary-500" on:click={onFormSubmit}>Add Template Group</button>
            </label>
        </div>
    </div>
{/if}