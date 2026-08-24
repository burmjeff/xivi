<script lang="ts">
	import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { createVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import {
		ChevronRight,
		Copy,
		Database,
		Layers3,
		Link2,
		ListPlus,
		LoaderCircle,
		Plus,
		RadioTower,
		RefreshCw,
		Search
	} from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { Paginated, SourceChannel, SourceGroup, StudioGroup } from '$lib/api/types';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';

	let {
		selectedGroup,
		selectedChannelId,
		selectedSourceId,
		actionKey,
		onSelectSource,
		onAddSource,
		onAttachSource,
		onSyncGroup,
		onCopyGroup,
		onAddGroup
	} = $props<{
		selectedGroup?: StudioGroup;
		selectedChannelId: number | null;
		selectedSourceId: number | null;
		actionKey: string | null;
		onSelectSource: (source: SourceChannel) => void;
		onAddSource: (source: SourceChannel) => void;
		onAttachSource: (source: SourceChannel) => void;
		onSyncGroup: (group: SourceGroup) => void;
		onCopyGroup: (group: SourceGroup) => void;
		onAddGroup: (group: SourceGroup) => void;
	}>();

	const client = useQueryClient();
	let sourceViewport: HTMLDivElement,
		searchInput = $state(''),
		searchQuery = $state(''),
		expandedGroupId = $state<number | null>(null),
		lastSearchValue = '';

	$effect(() => {
		const value = searchInput.trim();
		const timeout = setTimeout(() => (searchQuery = value), 180);
		return () => clearTimeout(timeout);
	});

	const groupsQuery = createQuery(() => ({
		queryKey: ['studio', 'source-groups'],
		queryFn: () => api<Paginated<SourceGroup>>('/api/v2/studio/source-groups?limit=500')
	}));
	const channelsQuery = createInfiniteQuery(() => ({
		queryKey: [
			'studio',
			'source-browser',
			searchQuery ? 'search' : 'group',
			searchQuery || expandedGroupId
		],
		enabled: Boolean(searchQuery || expandedGroupId),
		initialPageParam: '' as string,
		queryFn: ({ pageParam }) =>
			api<Paginated<SourceChannel>>(
				`/api/v2/studio/source-channels${params({
					q: searchQuery || undefined,
					group_id: searchQuery ? undefined : expandedGroupId,
					cursor: typeof pageParam === 'string' && pageParam ? pageParam : undefined,
					limit: 50
				})}`
			),
		getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined
	}));

	type SourceCollection = { id: number; name: string; groups: SourceGroup[] };
	type BrowserRow =
		| { key: string; kind: 'playlist'; collection: SourceCollection }
		| { key: string; kind: 'group'; group: SourceGroup }
		| { key: string; kind: 'channel'; channel: SourceChannel }
		| { key: string; kind: 'loading'; label: string }
		| { key: string; kind: 'empty'; label: string }
		| { key: string; kind: 'more' };

	let collections = $derived.by(() => {
		const map = new Map<number, SourceCollection>();
		for (const group of groupsQuery.data?.items ?? []) {
			let collection = map.get(group.playlist_id);
			if (!collection) {
				collection = { id: group.playlist_id, name: group.playlist_name, groups: [] };
				map.set(group.playlist_id, collection);
			}
			collection.groups.push(group);
		}
		return [...map.values()];
	});
	let loadedChannels = $derived(channelsQuery.data?.pages.flatMap((page) => page.items) ?? []);
	let rows = $derived.by((): BrowserRow[] => {
		const result: BrowserRow[] = [];
		if (searchInput.trim()) {
			if (channelsQuery.isPending || searchInput.trim() !== searchQuery) {
				return [{ key: 'search-loading', kind: 'loading', label: 'Searching channels…' }];
			}
			if (channelsQuery.isError) {
				return [
					{ key: 'search-error', kind: 'empty', label: 'Search results could not be loaded.' }
				];
			}
			for (const channel of loadedChannels)
				result.push({ key: `channel-${channel.id}`, kind: 'channel', channel });
			if (!result.length)
				result.push({
					key: 'search-empty',
					kind: 'empty',
					label: 'No source channels match this search.'
				});
			else if (channelsQuery.hasNextPage) result.push({ key: 'search-more', kind: 'more' });
			return result;
		}
		for (const collection of collections) {
			result.push({ key: `playlist-${collection.id}`, kind: 'playlist', collection });
			for (const group of collection.groups) {
				result.push({ key: `group-${group.id}`, kind: 'group', group });
				if (expandedGroupId !== group.id) continue;
				if (channelsQuery.isPending) {
					result.push({
						key: `group-${group.id}-loading`,
						kind: 'loading',
						label: 'Loading channels…'
					});
				} else if (channelsQuery.isError) {
					result.push({
						key: `group-${group.id}-error`,
						kind: 'empty',
						label: 'Channels could not be loaded.'
					});
				} else {
					for (const channel of loadedChannels)
						result.push({ key: `channel-${channel.id}`, kind: 'channel', channel });
					if (!loadedChannels.length)
						result.push({
							key: `group-${group.id}-empty`,
							kind: 'empty',
							label: 'This group has no channels.'
						});
					else if (channelsQuery.hasNextPage)
						result.push({ key: `group-${group.id}-more`, kind: 'more' });
				}
			}
		}
		return result;
	});

	function rowSize(row?: BrowserRow) {
		switch (row?.kind) {
			case 'playlist':
				return 38;
			case 'group':
				return 62;
			case 'channel':
				return 66;
			default:
				return 52;
		}
	}
	const sourceVirtualizer = createVirtualizer<HTMLDivElement, HTMLDivElement>({
		count: 0,
		getScrollElement: () => sourceViewport,
		estimateSize: (index) => rowSize(rows[index]),
		getItemKey: (index) => rows[index]?.key ?? index,
		overscan: 8
	});
	$effect(() => {
		const rowCount = rows.length;
		if (!sourceViewport) return;
		const instance = get(sourceVirtualizer);
		instance.setOptions({
			count: rowCount,
			getScrollElement: () => sourceViewport,
			estimateSize: (index) => rowSize(rows[index]),
			getItemKey: (index) => rows[index]?.key ?? index,
			overscan: 8
		});
		instance.measure();
	});
	let virtualRows = $derived(
		$sourceVirtualizer.getVirtualItems().filter((row) => row.index < rows.length)
	);
	$effect(() => {
		const last = virtualRows.at(-1);
		if (
			last &&
			last.index >= rows.length - 3 &&
			channelsQuery.hasNextPage &&
			!channelsQuery.isFetchingNextPage
		)
			void channelsQuery.fetchNextPage();
	});
	$effect(() => {
		const searchValue = searchInput.trim();
		if (sourceViewport && searchValue !== lastSearchValue) sourceViewport.scrollTop = 0;
		lastSearchValue = searchValue;
	});

	function toggleGroup(group: SourceGroup) {
		if (!group.enabled || group.channel_count === 0) return;
		expandedGroupId = expandedGroupId === group.id ? null : group.id;
	}
	async function refresh() {
		await Promise.all([
			client.invalidateQueries({ queryKey: ['studio', 'source-groups'] }),
			client.invalidateQueries({ queryKey: ['studio', 'source-browser'] })
		]);
	}
</script>

<div class="source-browser">
	<header>
		<div>
			<p class="eyebrow">Source browser</p>
			<h2>Sources</h2>
		</div>
		<button onclick={refresh} aria-label="Refresh source browser" title="Refresh source browser"
			><RefreshCw size={17} /></button
		>
	</header>
	<label class="source-search">
		<Search size={17} /><span class="sr-only">Search all source channels</span><input
			bind:value={searchInput}
			placeholder="Search all channels"
		/>
	</label>
	<div class="source-mode">
		{#if searchInput.trim()}<span><Search size={13} />Results across every source group</span
			>{:else}<span><Layers3 size={13} />Groups are collapsed until opened</span>{/if}
	</div>
	<div class="source-list" bind:this={sourceViewport} aria-label="Source groups and channels">
		{#if groupsQuery.isPending && !searchInput.trim()}
			{#each Array(7) as _}<div class="source-skeleton skeleton"></div>{/each}
		{:else if groupsQuery.isError && !searchInput.trim()}
			<div class="source-empty">Source groups could not be loaded.</div>
		{:else if !rows.length}
			<div class="source-empty">No source groups are available.</div>
		{:else}
			<div class="source-virtual-space" style={`height:${$sourceVirtualizer.getTotalSize()}px`}>
				{#each virtualRows as virtualRow (virtualRow.key)}
					{@const row = rows[virtualRow.index]}
					<div
						class="source-virtual-row"
						style={`height:${virtualRow.size}px;transform:translateY(${virtualRow.start}px)`}
					>
						{#if row.kind === 'playlist'}
							<div class="playlist-row">
								<Database size={14} /><strong>{row.collection.name}</strong><span
									>{row.collection.groups.length} group{row.collection.groups.length === 1
										? ''
										: 's'}</span
								>
							</div>
						{:else if row.kind === 'group'}
							<div
								class:disabled={!row.group.enabled || row.group.channel_count === 0}
								class="source-group-row"
							>
								<button
									class="group-disclosure"
									aria-expanded={expandedGroupId === row.group.id}
									disabled={!row.group.enabled || row.group.channel_count === 0}
									onclick={() => toggleGroup(row.group)}
								>
									<ChevronRight
										class={expandedGroupId === row.group.id ? 'expanded' : undefined}
										size={16}
									/>
									<span
										><strong>{row.group.name}</strong><small
											>{row.group.channel_count} channel{row.group.channel_count === 1
												? ''
												: 's'}{row.group.linked_group_count
												? ` · ${row.group.linked_group_count} synced`
												: ''}</small
										></span
									>
								</button>
								<div class="group-actions">
									<button
										disabled={!row.group.enabled ||
											row.group.channel_count === 0 ||
											actionKey !== null}
										onclick={() => onSyncGroup(row.group)}
										aria-label={`Continuously sync ${row.group.name}`}
										title="Keep a lineup group synchronized with this source group"
										><RadioTower size={13} />Sync</button
									><button
										disabled={!row.group.enabled ||
											row.group.channel_count === 0 ||
											actionKey !== null}
										onclick={() => onCopyGroup(row.group)}
										aria-label={`Copy ${row.group.name} once`}
										title="Create a one-time manual copy as a new lineup group"
										>{#if actionKey === `copy:${row.group.id}`}<LoaderCircle
												class="spin"
												size={13}
											/>{:else}<Copy size={13} />{/if}Copy</button
									><button
										disabled={!row.group.enabled ||
											row.group.channel_count === 0 ||
											!selectedGroup ||
											Boolean(selectedGroup.source_link) ||
											actionKey !== null}
										onclick={() => onAddGroup(row.group)}
										aria-label={`Add all channels from ${row.group.name} to ${selectedGroup?.name ?? 'the selected lineup group'}`}
										title={selectedGroup?.source_link
											? 'Synced groups control their own membership'
											: selectedGroup
												? `Add every missing channel to ${selectedGroup.name}`
												: 'Select a manual lineup group first'}
										>{#if actionKey === `add:${row.group.id}`}<LoaderCircle
												class="spin"
												size={13}
											/>{:else}<ListPlus size={13} />{/if}Add all</button
									>
								</div>
							</div>
						{:else if row.kind === 'channel'}
							<div class:selected={row.channel.id === selectedSourceId} class="source-channel-row">
								<button class="channel-select" onclick={() => onSelectSource(row.channel)}>
									<LogoTile src={row.channel.logo_url} name={row.channel.name} size="sm" contrast />
									<span
										><strong>{row.channel.name}</strong><small
											>{row.channel.playlist_name} › {row.channel.group_name}</small
										></span
									><em class:off={!row.channel.enabled}>{row.channel.enabled ? 'On' : 'Off'}</em>
								</button>
								<div class="channel-actions">
									<button
										disabled={!selectedGroup || Boolean(selectedGroup.source_link)}
										onclick={() => onAddSource(row.channel)}
										aria-label={`Add ${row.channel.name} to ${selectedGroup?.name ?? 'the lineup'}`}
										title="Add as a new lineup channel"><Plus size={14} /></button
									>
									{#if selectedChannelId}<button
											onclick={() => onAttachSource(row.channel)}
											aria-label={`Use ${row.channel.name} as a source variant`}
											title="Attach to selected lineup channel"><Link2 size={14} /></button
										>{/if}
								</div>
							</div>
						{:else if row.kind === 'loading'}
							<div class="source-status"><LoaderCircle class="spin" size={16} />{row.label}</div>
						{:else if row.kind === 'more'}
							<button class="load-more" onclick={() => channelsQuery.fetchNextPage()}
								>{#if channelsQuery.isFetchingNextPage}<LoaderCircle
										class="spin"
										size={15}
									/>Loading more…{:else}Load more channels{/if}</button
							>
						{:else}
							<div class="source-status">{row.label}</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.source-browser {
		display: flex;
		min-height: 0;
		height: 100%;
		flex-direction: column;
	}
	header {
		display: flex;
		min-height: 4.2rem;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 0.7rem 1rem;
	}
	header h2 {
		margin: 0;
		font-size: 1.2rem;
	}
	header button {
		display: grid;
		width: 2.4rem;
		height: 2.4rem;
		place-items: center;
		border: 0;
		border-radius: 0.7rem;
		background: var(--surface-raised);
		color: var(--text);
		cursor: pointer;
	}
	.source-search {
		display: flex;
		min-height: 2.7rem;
		align-items: center;
		gap: 0.45rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		margin: 0.65rem 0.65rem 0.4rem;
		background: var(--surface-raised);
		padding: 0 0.65rem;
		color: var(--muted);
	}
	.source-search input {
		min-width: 0;
		width: 100%;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
		font-size: 0.75rem;
	}
	.source-mode {
		min-height: 1.55rem;
		border-bottom: 1px solid var(--line);
		padding: 0 0.7rem 0.4rem;
		color: var(--muted);
		font-size: 0.58rem;
	}
	.source-mode span {
		display: flex;
		align-items: center;
		gap: 0.3rem;
	}
	.source-list {
		position: relative;
		min-height: 0;
		flex: 1;
		overflow-y: auto;
		overscroll-behavior: contain;
	}
	.source-virtual-space {
		position: relative;
		width: 100%;
	}
	.source-virtual-row {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
	}
	.playlist-row {
		display: grid;
		height: 100%;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.4rem;
		border-bottom: 1px solid var(--line);
		background: var(--deep);
		padding: 0 0.7rem;
		color: var(--aqua);
	}
	.playlist-row strong {
		overflow: hidden;
		font-size: 0.68rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.playlist-row span {
		color: var(--muted);
		font-size: 0.55rem;
	}
	.source-group-row {
		position: relative;
		display: flex;
		height: 100%;
		align-items: center;
		border-bottom: 1px solid var(--line);
		background: var(--surface);
		padding: 0.3rem 0.45rem;
	}
	.source-group-row.disabled {
		opacity: 0.48;
	}
	.group-disclosure {
		display: grid;
		min-width: 0;
		width: 100%;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: center;
		gap: 0.35rem;
		border: 0;
		background: transparent;
		padding: 0 0.15rem;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}
	.group-disclosure :global(svg) {
		transition: transform var(--motion-fast) var(--ease-out);
	}
	.group-disclosure :global(svg.expanded) {
		transform: rotate(90deg);
	}
	.group-disclosure span {
		display: grid;
		min-width: 0;
	}
	.group-disclosure strong,
	.group-disclosure small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.group-disclosure strong {
		font-size: 0.7rem;
	}
	.group-disclosure small {
		color: var(--muted);
		font-size: 0.57rem;
	}
	.group-actions {
		position: absolute;
		top: 50%;
		right: 0.45rem;
		display: none;
		grid-template-columns: repeat(3, 2rem);
		gap: 0.25rem;
		transform: translateY(-50%);
		background: var(--surface);
		padding-left: 0.4rem;
	}
	.source-group-row:hover .group-actions,
	.source-group-row:focus-within .group-actions {
		display: grid;
	}
	.group-actions button,
	.channel-actions button,
	.load-more {
		display: flex;
		min-height: 1.85rem;
		align-items: center;
		justify-content: center;
		gap: 0.25rem;
		border: 1px solid var(--line);
		border-radius: 0.5rem;
		background: var(--surface-raised);
		color: var(--text);
		font-size: 0.56rem;
		font-weight: 750;
		cursor: pointer;
	}
	.group-actions button:disabled,
	.channel-actions button:disabled {
		opacity: 0.42;
		cursor: not-allowed;
	}
	.group-actions button {
		width: 2rem;
		padding: 0;
		font-size: 0;
	}
	.source-channel-row {
		position: relative;
		display: flex;
		height: 100%;
		align-items: center;
		border-bottom: 1px solid var(--line);
		background: var(--surface);
		padding: 0.4rem 0.45rem 0.4rem 1.15rem;
	}
	.source-channel-row:hover,
	.source-channel-row:focus-within,
	.source-channel-row.selected {
		background: color-mix(in oklch, var(--aqua) 9%, var(--surface));
	}
	.channel-select {
		display: grid;
		min-width: 0;
		width: 100%;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.5rem;
		border: 0;
		background: transparent;
		padding: 0;
		color: inherit;
		text-align: left;
		cursor: pointer;
	}
	.channel-select > span {
		display: grid;
		min-width: 0;
	}
	.channel-select strong,
	.channel-select small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.channel-select strong {
		font-size: 0.68rem;
	}
	.channel-select small {
		color: var(--muted);
		font-size: 0.55rem;
	}
	.channel-select em {
		border-radius: 999px;
		background: color-mix(in oklch, var(--success) 18%, transparent);
		padding: 0.18rem 0.35rem;
		color: var(--success);
		font-size: 0.52rem;
		font-style: normal;
		font-weight: 800;
	}
	.channel-select em.off {
		background: var(--surface-raised);
		color: var(--muted);
	}
	.channel-actions {
		position: absolute;
		right: 0.45rem;
		display: none;
		gap: 0.25rem;
		background: inherit;
		padding-left: 0.35rem;
	}
	.source-channel-row:hover .channel-actions,
	.source-channel-row:focus-within .channel-actions,
	.source-channel-row.selected .channel-actions {
		display: flex;
	}
	.channel-actions button {
		width: 1.9rem;
		padding: 0;
	}
	.source-status,
	.source-empty,
	.load-more {
		display: flex;
		height: 100%;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		color: var(--muted);
		font-size: 0.65rem;
	}
	.source-empty {
		min-height: 10rem;
		padding: 1rem;
		text-align: center;
	}
	.load-more {
		width: calc(100% - 0.9rem);
		margin: 0.35rem 0.45rem;
	}
	.source-skeleton {
		height: 4rem;
		margin: 0.45rem;
		border-radius: 0.6rem;
	}
	:global(.spin) {
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.group-disclosure :global(svg) {
			transition: none;
		}
		:global(.spin) {
			animation: none;
		}
	}
	@media (hover: none) {
		.group-disclosure {
			padding-right: 7rem;
		}
		.group-actions {
			display: grid;
		}
	}
</style>
