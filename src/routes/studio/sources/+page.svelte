<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { createTable, coreFeatures, tableFeatures } from '@tanstack/svelte-table';
	import {
		Plus,
		RefreshCw,
		Trash2,
		Pencil,
		Search,
		Radio,
		CheckCircle2,
		XCircle,
		Power,
		PowerOff
	} from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { LegacyPlaylist, Paginated, SourceChannel } from '$lib/api/types';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';

	type PlaylistResponse = { playlists: LegacyPlaylist[]; count: number };
	const client = useQueryClient(),
		features = tableFeatures({ ...coreFeatures });
	const playlistsQuery = createQuery(() => ({
		queryKey: ['studio', 'sources'],
		queryFn: () => api<PlaylistResponse>('/api/playlists')
	}));
	let selectedPlaylistId = $state<number | null>(null),
		channelSearch = $state(''),
		selectedChannels = $state(new Set<number>()),
		name = $state(''),
		url = $state(''),
		editing = $state<LegacyPlaylist | null>(null),
		busy = $state<number | 'form' | null>(null),
		message = $state('');
	$effect(() => {
		if (!selectedPlaylistId && playlistsQuery.data?.playlists[0])
			selectedPlaylistId = playlistsQuery.data.playlists[0].id;
	});
	const channelsQuery = createQuery(() => ({
		queryKey: ['studio', 'source-channels', selectedPlaylistId, channelSearch],
		enabled: !!selectedPlaylistId,
		queryFn: () =>
			api<Paginated<SourceChannel>>(
				`/api/v2/studio/source-channels${params({ playlist_id: selectedPlaylistId, q: channelSearch, limit: 500 })}`
			)
	}));
	const table = createTable({
		features,
		columns: [
			{ accessorKey: 'name', header: 'Source' },
			{ accessorKey: 'url', header: 'Playlist URL' },
			{ accessorKey: 'updated_at', header: 'Last updated' }
		],
		get data() {
			return playlistsQuery.data?.playlists ?? [];
		}
	});

	function edit(source: LegacyPlaylist) {
		editing = source;
		name = source.name;
		url = source.url;
	}
	function reset() {
		editing = null;
		name = '';
		url = '';
	}
	async function save() {
		if (!name.trim() || !url.trim()) return;
		busy = 'form';
		message = '';
		const wasEditing = !!editing;
		try {
			await api('/api/playlist', {
				method: editing ? 'PUT' : 'POST',
				body: JSON.stringify({
					...(editing ? { id: editing.id } : {}),
					name: name.trim(),
					url: url.trim(),
					created_at: editing?.created_at,
					updated_at: new Date().toISOString()
				})
			});
			reset();
			await client.invalidateQueries({ queryKey: ['studio', 'sources'] });
			message = wasEditing
				? 'Source updated.'
				: 'Source added. Import is running in the background.';
		} catch {
			message = 'The source could not be saved. Check the URL and name.';
		} finally {
			busy = null;
		}
	}
	async function refresh(source: LegacyPlaylist) {
		busy = source.id;
		try {
			await api(`/api/v2/studio/sources/${source.id}/refresh`, { method: 'POST' });
			message = `Refresh queued for ${source.name}.`;
			setTimeout(() => client.invalidateQueries({ queryKey: ['studio', 'source-channels'] }), 2500);
		} catch {
			message = 'The refresh could not be started.';
		} finally {
			busy = null;
		}
	}
	async function remove(source: LegacyPlaylist) {
		if (
			!confirm(
				`Delete “${source.name}” and its imported groups and channels? Lineup channels remain unless their source mapping is removed.`
			)
		)
			return;
		busy = source.id;
		try {
			await api(`/api/playlist/${source.id}`, { method: 'DELETE' });
			if (selectedPlaylistId === source.id) selectedPlaylistId = null;
			await client.invalidateQueries({ queryKey: ['studio', 'sources'] });
			message = 'Source deleted.';
		} catch {
			message = 'The source could not be deleted.';
		} finally {
			busy = null;
		}
	}
	function toggleChannel(id: number) {
		const next = new Set(selectedChannels);
		next.has(id) ? next.delete(id) : next.add(id);
		selectedChannels = next;
	}
	async function setEnabled(enabled: boolean) {
		if (!selectedChannels.size) return;
		try {
			await api('/api/v2/studio/source-channels/enabled', {
				method: 'PATCH',
				body: JSON.stringify({ channel_ids: [...selectedChannels], enabled })
			});
			selectedChannels = new Set();
			await client.invalidateQueries({ queryKey: ['studio', 'source-channels'] });
			message = `${enabled ? 'Enabled' : 'Disabled'} selected channels.`;
		} catch {
			message = 'Enablement could not be updated.';
		}
	}
	function format(value: string) {
		return value
			? new Intl.DateTimeFormat([], { dateStyle: 'medium', timeStyle: 'short' }).format(
					new Date(value)
				)
			: 'Never';
	}
</script>

<svelte:head><title>Sources · Xivi Studio</title></svelte:head>
<StudioHeader
	title="Sources"
	description="Bring in M3U playlists, inspect their groups and channels, and keep every import healthy."
/>
{#if message}<div class="source-message" role="status">
		{message}<button onclick={() => (message = '')}>Dismiss</button>
	</div>{/if}
<div class="sources-layout">
	<section class="panel source-editor">
		<header>
			<p class="eyebrow">{editing ? 'Edit source' : 'Add source'}</p>
			<h2>{editing ? editing.name : 'Connect a playlist'}</h2>
		</header>
		<form
			onsubmit={(event) => {
				event.preventDefault();
				void save();
			}}
		>
			<label>Source name<input bind:value={name} placeholder="Living room TV" required /></label
			><label
				>M3U URL<input
					bind:value={url}
					type="url"
					placeholder="https://provider.example/playlist.m3u"
					required
				/></label
			>
			<div>
				<button class="app-button app-button--primary" disabled={busy === 'form'}
					><Plus size={17} />{editing ? 'Save source' : 'Add and import'}</button
				>{#if editing}<button type="button" class="app-button app-button--quiet" onclick={reset}
						>Cancel</button
					>{/if}
			</div>
		</form>
		<aside>
			<Radio size={18} />
			<p>
				Imports run in the background. Existing channel mappings are retained where identities
				match.
			</p>
		</aside>
	</section>
	<section class="panel sources-table">
		<header>
			<div>
				<p class="eyebrow">Connected</p>
				<h2>{table.getRowModel().rows.length} playlist sources</h2>
			</div>
		</header>
		{#if playlistsQuery.isPending}<div
				class="table-loading skeleton"
			></div>{:else if !table.getRowModel().rows.length}<div class="table-empty">
				<h3>No playlist sources</h3>
				<p>Add an M3U URL to begin.</p>
			</div>{:else}<div class="table-scroll">
				<table>
					<thead
						><tr
							><th>Source</th><th>Playlist URL</th><th>Last update</th><th
								><span class="sr-only">Actions</span></th
							></tr
						></thead
					><tbody
						>{#each table.getRowModel().rows as row}{@const source = row.original}<tr
								class:selected={selectedPlaylistId === source.id}
								><td
									><button
										class="source-choose"
										onclick={() => {
											selectedPlaylistId = source.id;
											selectedChannels = new Set();
										}}
										><strong>{source.name}</strong><small
											><CheckCircle2 size={13} /> Connected</small
										></button
									></td
								><td><span class="url">{source.url}</span></td><td class="tabular"
									>{format(source.updated_at)}</td
								><td
									><div class="row-actions">
										<button
											onclick={() => refresh(source)}
											disabled={busy === source.id}
											aria-label={`Refresh ${source.name}`}
											><RefreshCw
												class={busy === source.id ? 'spin' : undefined}
												size={17}
											/></button
										><button onclick={() => edit(source)} aria-label={`Edit ${source.name}`}
											><Pencil size={17} /></button
										><button onclick={() => remove(source)} aria-label={`Delete ${source.name}`}
											><Trash2 size={17} /></button
										>
									</div></td
								></tr
							>{/each}</tbody
					>
				</table>
			</div>{/if}
	</section>
	<section class="panel source-catalog">
		<header>
			<div>
				<p class="eyebrow">Catalog inspector</p>
				<h2>
					{playlistsQuery.data?.playlists.find((source) => source.id === selectedPlaylistId)
						?.name ?? 'Select a source'}
				</h2>
			</div>
			{#if selectedChannels.size}<div class="batch-actions">
					<span>{selectedChannels.size} selected</span><button onclick={() => setEnabled(true)}
						><Power size={15} />Enable</button
					><button onclick={() => setEnabled(false)}><PowerOff size={15} />Disable</button>
				</div>{/if}
		</header>
		<label class="catalog-search"
			><Search size={18} /><input
				bind:value={channelSearch}
				placeholder="Server-side channel search"
			/></label
		>{#if channelsQuery.isPending}<div class="catalog-loading">
				{#each Array(8) as _}<div class="skeleton"></div>{/each}
			</div>{:else if !channelsQuery.data?.total}<div class="table-empty">
				<XCircle size={24} />
				<h3>No imported channels</h3>
				<p>Refresh this source, then check its last error if the catalog stays empty.</p>
			</div>{:else}<div class="catalog-list">
				{#each channelsQuery.data?.items ?? [] as channel}<label
						><input
							type="checkbox"
							checked={selectedChannels.has(channel.id)}
							onchange={() => toggleChannel(channel.id)}
						/><span
							><strong>{channel.name}</strong><small
								>{channel.group_name} · {channel.tvg_id || 'No TVG ID'}</small
							></span
						><em class:off={!channel.enabled}>{channel.enabled ? 'Enabled' : 'Disabled'}</em></label
					>{/each}
			</div>{/if}
	</section>
</div>

<style>
	.source-message {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin: 0 2.5rem 1rem;
		border-radius: 0.8rem;
		background: var(--sun);
		padding: 0.7rem 1rem;
		color: var(--ink);
		font-size: 0.75rem;
		font-weight: 750;
	}
	.source-message button {
		border: 0;
		background: transparent;
		text-decoration: underline;
		cursor: pointer;
	}
	.sources-layout {
		display: grid;
		grid-template-columns: minmax(18rem, 0.65fr) minmax(30rem, 1.35fr);
		gap: 1rem;
		padding: 0 clamp(1.4rem, 3vw, 2.5rem) 3rem;
	}
	.source-catalog {
		grid-column: 1/3;
	}
	.panel {
		overflow: hidden;
	}
	.panel > header {
		display: flex;
		min-height: 4.6rem;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 0.9rem 1rem;
	}
	.panel h2 {
		margin: 0;
		font-size: 1.3rem;
	}
	.source-editor form {
		display: grid;
		gap: 0.8rem;
		padding: 1rem;
	}
	.source-editor label {
		display: grid;
		gap: 0.3rem;
		color: var(--muted);
		font-size: 0.7rem;
	}
	.source-editor input {
		min-height: 2.75rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface-raised);
		padding: 0.55rem 0.7rem;
		color: var(--text);
	}
	.source-editor form > div {
		display: flex;
		gap: 0.4rem;
	}
	.source-editor aside {
		display: flex;
		align-items: flex-start;
		gap: 0.6rem;
		border-top: 1px solid var(--line);
		padding: 1rem;
		color: var(--muted);
	}
	.source-editor aside p {
		margin: 0;
		font-size: 0.7rem;
	}
	.table-scroll {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th,
	td {
		border-bottom: 1px solid var(--line);
		padding: 0.75rem 0.9rem;
		text-align: left;
		font-size: 0.72rem;
	}
	th {
		color: var(--muted);
		font-size: 0.62rem;
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}
	tbody tr:hover,
	tbody tr.selected {
		background: color-mix(in oklch, var(--periwinkle) 10%, var(--surface));
	}
	td:first-child {
		display: grid;
	}
	.source-choose {
		display: grid;
		border: 0;
		background: transparent;
		padding: 0;
		color: inherit;
		text-align: left;
		cursor: pointer;
	}
	td small {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		color: var(--success);
		font-size: 0.6rem;
	}
	.url {
		display: block;
		max-width: 22rem;
		overflow: hidden;
		color: var(--muted);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.row-actions {
		display: flex;
		justify-content: flex-end;
	}
	.row-actions button {
		display: grid;
		width: 2.2rem;
		height: 2.2rem;
		place-items: center;
		border: 0;
		border-radius: 0.55rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.row-actions button:hover {
		background: var(--surface-raised);
		color: var(--text);
	}
	.table-loading {
		height: 18rem;
	}
	.table-empty {
		display: grid;
		min-height: 12rem;
		place-items: center;
		align-content: center;
		text-align: center;
		color: var(--muted);
	}
	.table-empty h3,
	.table-empty p {
		margin: 0.25rem;
	}
	.catalog-search {
		display: flex;
		min-height: 2.8rem;
		align-items: center;
		gap: 0.5rem;
		border-bottom: 1px solid var(--line);
		padding: 0 1rem;
		color: var(--muted);
	}
	.catalog-search input {
		width: 100%;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
	}
	.catalog-list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(22rem, 1fr));
		max-height: 32rem;
		overflow: auto;
	}
	.catalog-list label {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.6rem;
		min-height: 3.8rem;
		border-right: 1px solid var(--line);
		border-bottom: 1px solid var(--line);
		padding: 0.55rem 0.8rem;
	}
	.catalog-list input {
		accent-color: var(--periwinkle);
	}
	.catalog-list span {
		display: grid;
		min-width: 0;
	}
	.catalog-list strong,
	.catalog-list small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.catalog-list strong {
		font-size: 0.72rem;
	}
	.catalog-list small {
		color: var(--muted);
		font-size: 0.6rem;
	}
	.catalog-list em {
		border-radius: 99px;
		background: color-mix(in oklch, var(--success) 13%, transparent);
		padding: 0.2rem 0.4rem;
		color: var(--success);
		font-size: 0.58rem;
		font-style: normal;
	}
	.catalog-list em.off {
		background: var(--surface-raised);
		color: var(--muted);
	}
	.batch-actions {
		display: flex;
		align-items: center;
		gap: 0.35rem;
	}
	.batch-actions span {
		color: var(--muted);
		font-size: 0.65rem;
	}
	.batch-actions button {
		display: flex;
		min-height: 2.3rem;
		align-items: center;
		gap: 0.3rem;
		border: 1px solid var(--line);
		border-radius: 0.6rem;
		background: var(--surface-raised);
		padding: 0.35rem 0.55rem;
		font-size: 0.62rem;
		cursor: pointer;
	}
	.catalog-loading {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
	}
	.catalog-loading div {
		height: 3.8rem;
		border-bottom: 1px solid var(--line);
	}
	@media (max-width: 1000px) {
		.sources-layout {
			grid-template-columns: 1fr;
		}
		.source-catalog {
			grid-column: 1;
		}
	}
	@media (max-width: 600px) {
		.sources-layout {
			padding-inline: 1rem;
		}
		.source-message {
			margin-inline: 1rem;
		}
		.catalog-list {
			grid-template-columns: 1fr;
		}
		.batch-actions span {
			display: none;
		}
	}
</style>
