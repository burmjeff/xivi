<!-- Epg.svelte -->

<script lang="ts">
	import { onMount } from 'svelte';
	import {
		popup,
		getModalStore,
		type ModalSettings,
		type PopupSettings
	} from '@skeletonlabs/skeleton';
	import { epgs } from '@xivi/stores/epg_store';
	import Icon from '@iconify/svelte';

	const modalStore = getModalStore();

	const updateEpgs = async () => {
		const response = await fetch('/api/epgs');
		const data = await response.json();
		return data.epgs;
	};

	onMount(async () => {
		epgs.set(await updateEpgs());
	});

	let epgSettings: PopupSettings = {
		// Set the event as: click | hover | hover-click
		event: 'click',
		// Provide a matching 'data-popup' value.
		target: 'addEpgPopup'
	};

	const addEpgTooltip: PopupSettings = {
		event: 'hover',
		target: 'addEpgTooltip',
		placement: 'right'
	};

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
		let initialValue;
		if (type == "name") {
			initialValue =  $epgs[epgIdx].name
		} else if (type == "url") {
			initialValue =  $epgs[epgIdx].url
		}
		
		const prompt: ModalSettings = {
			type: 'prompt',
			title: 'Edit EPG',
			body: `Enter new EPG ${type} in field below.`,
			value: initialValue,
			valueAttr: { type: 'text', minlength: 1, maxlength: 50, required: true },
			response: (newValue: string) => {
				if (newValue) editEpg(epgIdx, type, newValue);
			},
			buttonTextCancel: 'Cancel',
			buttonTextSubmit: 'Submit'
		};
		modalStore.trigger(prompt);
	}

	async function editEpg(epgIdx: number, type: string, newValue: string) {
		if (type == "name") {
			$epgs[epgIdx].name = newValue
		} else if (type == "url") {
			$epgs[epgIdx].url = newValue
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

	function deletePrompt(epgId: number): void {
		const modal: ModalSettings = {
			type: 'confirm',
			title: 'Please Confirm',
			body: 'Are you sure you wish to delete this epg?',
			// TRUE if confirm pressed, FALSE if cancel pressed
			response: (r: boolean) => {
				if (r) deleteEpg(epgId);
			}
		};
		modalStore.trigger(modal);
	}

	async function deleteEpg(epgId: number) {
		try {
			const response = await fetch(`/api/epg/${epgId}`, {
				method: 'DELETE'
			});
			const data = await response.status;
			console.log('Deleted epg:', data);
			$epgs = $epgs.filter((t) => t.id != epgId);
			modalStore.close();
		} catch (error) {
			console.log('Error deleting epg:', error);
			return;
		}
	}
</script>

<section class="epgs card p-1">
	<header class="epgs-header flex items-center justify-center space-x-4">
		<h3 class="h3 font-bold">Epgs</h3>
		<button class="btn btn-md" use:popup={epgSettings} use:popup={addEpgTooltip}>
			<Icon icon="icon-park-twotone:add-one" color="#0a7e85" width="25" height="25" />
		</button>
	</header>
	<div id="accord" class="epgs-viewport min-w-full overflow-auto">
		{#if $epgs != null}
			<table class="epgTable table table-hover">
				<thead class="items-center text-center">
					<tr id="thead">
						<th>Name</th>
						<th>Url</th>
						<th>Updated</th>
						<th>Delete</th>
					</tr>
				</thead>
				<tbody class="items-center">
					{#if $epgs.length > 0}
						{#each $epgs as epg, epgIdx (epg.id)}
							<tr>
								<td>
									{epg.name}
									<button
										class="btn-icon btn-icon-sm inset-y-0 !bg-transparent"
										on:click={() => {
											(epg.itemOpen = true), editPrompt(epgIdx, "name");
										}}
									>
										<Icon icon="icon-park-outline:edit-one" width="18" height="18" />
									</button>
								</td>
								<td>
									{epg.url}
									<button
										class="btn-icon btn-icon-sm inset-y-0 !bg-transparent"
										on:click={() => {
											(epg.itemOpen = true), editPrompt(epgIdx, "url");
										}}
									>
										<Icon icon="icon-park-outline:edit-one" width="18" height="18" />
									</button>
								</td>
								<td>
									{epg.updated_at}
								</td>
								<td
									><button
										class="btn-icon btn-icon-sm inset-y-0 !bg-transparent"
										on:click={() => {
											(epg.itemOpen = true), deletePrompt(epg.id);
										}}
									>
										<Icon icon="icon-park-outline:delete" width="18" height="18" />
									</button>
								</td>
							</tr>
						{/each}
					{:else}
						<p>No epgs found</p>
					{/if}
				</tbody>
			</table>
		{/if}
	</div>
</section>

<div class="card gap-4 p-4" data-popup="addEpgPopup">
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
			<button class="btn bg-primary-500" on:click={addEpg}>Add Epg</button>
		</label>
	</div>
</div>

<div class="card variant-filled-secondary p-2" data-popup="addEpgTooltip">
	<p>Add New EPG</p>
	<div class="variant-filled-secondary arrow" />
</div>

<style>
	#accord {
		max-height: 82vh;
		height: 82vh;
	}
	#thead {
		position: relative;
		text-align: center;
	}
</style>
