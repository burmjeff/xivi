<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { Dialog } from 'bits-ui';
	import {
		Search,
		Plus,
		RefreshCw,
		UploadCloud,
		Trash2,
		Undo2,
		ChevronRight,
		PanelRightClose,
		LockKeyhole,
		CircleGauge,
		History,
		Unlink,
		MoreHorizontal,
		Pencil,
		ChevronDown,
		Check,
		RadioTower,
		X,
		CircleAlert
	} from '@lucide/svelte';
	import { api, params, XiviAPIError } from '$lib/api/client';
	import type {
		LineupSummary,
		MatchRejection,
		MatchReview,
		Paginated,
		SourceChannel,
		SourceGroup,
		SourceGroupImportResult,
		StudioGroup,
		WorkspaceChannel
	} from '$lib/api/types';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import SourceBrowser from '$lib/components/studio/SourceBrowser.svelte';
	import WorkspaceRow from '$lib/components/studio/WorkspaceRow.svelte';

	const lineupId = Number(page.params.id),
		client = useQueryClient();
	const lineupsQuery = createQuery(() => ({
		queryKey: ['studio', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/studio/lineups')
	}));
	const groupsQuery = createQuery(() => ({
		queryKey: ['studio', 'lineup-groups', lineupId],
		queryFn: () => api<Paginated<StudioGroup>>(`/api/v2/studio/lineups/${lineupId}/groups`),
		refetchInterval: 5000
	}));
	let lineup = $derived(lineupsQuery.data?.items.find((item) => item.id === lineupId));
	let selectedGroupId = $state<number | null>(Number(page.url.searchParams.get('group')) || null),
		selectedChannelId = $state<number | null>(null),
		selectedSourceId = $state<number | null>(null),
		channelSearch = $state(''),
		inspectorOpen = $state(true),
		selectedIds = $state(new Set<number>()),
		batchTargetGroupId = $state(0),
		message = $state(''),
		syncDialogOpen = $state(false),
		syncGroup = $state<StudioGroup | null>(null),
		syncSourceGroupId = $state(0),
		syncGroupName = $state(''),
		syncSearch = $state(''),
		syncError = $state(''),
		followGroupName = $state(true),
		followChannelNames = $state(true),
		syncSaving = $state(false),
		sourceGroupAction = $state<string | null>(null),
		undo = $state<{
			source: number;
			target: number;
			placement: 'before' | 'after';
			label: string;
		} | null>(null);
	$effect(() => {
		if (!selectedGroupId && groupsQuery.data?.items[0])
			selectedGroupId = groupsQuery.data.items[0].id;
	});
	let selectedGroup = $derived(
		groupsQuery.data?.items.find((group) => group.id === selectedGroupId)
	);
	let selectedGroupManaged = $derived(Boolean(selectedGroup?.source_link));
	const channelKey = $derived([
		'studio',
		'group-channels',
		selectedGroupId,
		channelSearch
	] as const);
	const channelsQuery = createQuery(() => ({
		queryKey: channelKey,
		enabled: !!selectedGroupId,
		queryFn: () =>
			api<Paginated<WorkspaceChannel>>(
				`/api/v2/studio/groups/${selectedGroupId}/channels${params({ q: channelSearch, limit: 500 })}`
			)
	}));
	const sourceGroupsQuery = createQuery(() => ({
		queryKey: ['studio', 'source-groups'],
		enabled: syncDialogOpen,
		queryFn: () => api<Paginated<SourceGroup>>('/api/v2/studio/source-groups?limit=500')
	}));
	let matchingSourceGroups = $derived.by(() => {
		const query = syncSearch.trim().toLocaleLowerCase();
		return (sourceGroupsQuery.data?.items ?? []).filter(
			(group) =>
				!query ||
				group.name.toLocaleLowerCase().includes(query) ||
				group.playlist_name.toLocaleLowerCase().includes(query)
		);
	});
	type Logo = { id: number; name: string; image: string };
	type LogosResponse = { logos: Logo[] };
	const logosQuery = createQuery(() => ({
		queryKey: ['logos'],
		queryFn: () => api<LogosResponse>('/api/logos')
	}));
	const variantsQuery = createQuery(() => ({
		queryKey: ['studio', 'channel-variants', selectedChannelId],
		enabled: !!selectedChannelId,
		queryFn: () =>
			api<Paginated<MatchReview>>(`/api/v2/studio/channels/${selectedChannelId}/matches`)
	}));
	const rejectionsQuery = createQuery(() => ({
		queryKey: ['studio', 'channel-rejections', selectedChannelId],
		enabled: !!selectedChannelId,
		queryFn: () =>
			api<Paginated<MatchRejection>>(`/api/v2/studio/channels/${selectedChannelId}/rejections`)
	}));
	let selectedChannel = $derived(
		channelsQuery.data?.items.find((item) => item.id === selectedChannelId)
	);
	let editName = $state(''),
		editTvg = $state(''),
		editLogo = $state(0),
		logoSearch = $state(''),
		logoPickerOpen = $state(false),
		editorId = $state<number | null>(null),
		saving = $state(false);
	let selectedLogo = $derived((logosQuery.data?.logos ?? []).find((logo) => logo.id === editLogo));
	let matchingLogos = $derived.by(() => {
		const query = logoSearch.trim().toLocaleLowerCase();
		return (logosQuery.data?.logos ?? [])
			.filter((logo) => !query || logo.name.toLocaleLowerCase().includes(query))
			.sort((a, b) => Number(b.id === editLogo) - Number(a.id === editLogo));
	});
	let visibleLogos = $derived(matchingLogos.slice(0, 48));
	$effect(() => {
		if (selectedChannel && selectedChannel.id !== editorId) {
			editorId = selectedChannel.id;
			editName = selectedChannel.name;
			editTvg = selectedChannel.tvg_id ?? '';
			editLogo = selectedChannel.logo_id;
			logoSearch = '';
			logoPickerOpen = false;
		}
	});
	function logoSrc(image?: string) {
		if (!image) return undefined;
		return image.startsWith('/') ? image : `/${image}`;
	}
	function requestError(error: unknown, fallback: string) {
		return error instanceof XiviAPIError ? error.message : fallback;
	}
	function openSyncSettings(group: StudioGroup | null) {
		syncGroup = group;
		syncSourceGroupId = group?.source_link?.source_group_id ?? 0;
		syncGroupName = group?.name ?? '';
		syncSearch = '';
		syncError = '';
		followGroupName = group?.source_link?.follow_group_name ?? true;
		followChannelNames = group?.source_link?.follow_channel_names ?? true;
		syncDialogOpen = true;
	}
	function chooseSourceGroup(group: SourceGroup) {
		syncSourceGroupId = group.id;
		syncError = '';
		if (!syncGroup && !syncGroupName.trim()) syncGroupName = group.name;
	}
	async function saveSyncSettings() {
		if (!syncSourceGroupId || (!syncGroup && !syncGroupName.trim())) return;
		syncError = '';
		syncSaving = true;
		try {
			let groupID = syncGroup?.id;
			if (!groupID) {
				const response = await api<{ group_id: number; job_id: number; status: string }>(
					`/api/v2/studio/lineups/${lineupId}/groups`,
					{
						method: 'POST',
						body: JSON.stringify({
							name: syncGroupName.trim(),
							source_link: {
								source_group_id: syncSourceGroupId,
								follow_group_name: followGroupName,
								follow_channel_names: followChannelNames
							}
						})
					}
				);
				groupID = response.group_id;
				selectedGroupId = groupID;
			} else {
				await api(`/api/v2/studio/groups/${groupID}/source-link`, {
					method: 'PUT',
					body: JSON.stringify({
						source_group_id: syncSourceGroupId,
						follow_group_name: followGroupName,
						follow_channel_names: followChannelNames
					})
				});
			}
			syncDialogOpen = false;
			selectedChannelId = null;
			selectedIds = new Set();
			await refreshWorkspace();
			message = 'The source connection is saved and its first sync is running.';
		} catch (error) {
			syncError = requestError(error, 'The source group could not be connected.');
		} finally {
			syncSaving = false;
		}
	}
	async function syncNow(group: StudioGroup) {
		if (!group.source_link) return;
		try {
			await api(`/api/v2/studio/groups/${group.id}/sync`, { method: 'POST' });
			message = `Syncing “${group.name}”…`;
			await refreshWorkspace();
		} catch {
			message = 'The group sync could not be started.';
		}
	}
	async function disconnectSync(retainChannels: boolean) {
		if (!syncGroup?.source_link) return;
		const action = retainChannels ? 'keep its current channels' : 'remove its synced channels';
		if (!confirm(`Disconnect “${syncGroup.name}” and ${action}?`)) return;
		syncSaving = true;
		try {
			await api(
				`/api/v2/studio/groups/${syncGroup.id}/source-link?retain_channels=${retainChannels}`,
				{ method: 'DELETE' }
			);
			syncDialogOpen = false;
			selectedChannelId = null;
			selectedIds = new Set();
			await refreshWorkspace();
			message = retainChannels
				? 'Source disconnected. The current channels are now manually managed.'
				: 'Source disconnected and its synced channels were removed.';
		} catch {
			message = 'The source connection could not be removed.';
		} finally {
			syncSaving = false;
		}
	}

	async function refreshWorkspace() {
		await Promise.all([
			client.invalidateQueries({ queryKey: ['studio', 'lineup-groups', lineupId] }),
			client.invalidateQueries({ queryKey: ['studio', 'group-channels'] }),
			client.invalidateQueries({ queryKey: ['studio', 'source-browser'] }),
			client.invalidateQueries({ queryKey: ['studio', 'source-groups'] })
		]);
	}
	async function move(
		sourceId: number,
		targetId: number,
		placement: 'before' | 'after',
		recordUndo = true
	) {
		if (!selectedGroupId || selectedGroupManaged || sourceId === targetId) return;
		const key = channelKey,
			previous = client.getQueryData<Paginated<WorkspaceChannel>>(key);
		if (!previous) return;
		const source = previous.items.find((item) => item.id === sourceId),
			oldIndex = previous.items.findIndex((item) => item.id === sourceId),
			oldTarget = oldIndex === 0 ? previous.items[1] : previous.items[oldIndex - 1],
			oldPlacement: 'before' | 'after' = oldIndex === 0 ? 'before' : 'after';
		const items = previous.items.filter((item) => item.id !== sourceId),
			targetIndex = items.findIndex((item) => item.id === targetId);
		items.splice(Math.max(0, targetIndex + (placement === 'after' ? 1 : 0)), 0, source!);
		client.setQueryData(key, { ...previous, items });
		try {
			await api(`/api/v2/studio/groups/${selectedGroupId}/channels/${sourceId}/position`, {
				method: 'PATCH',
				body: JSON.stringify(
					placement === 'before' ? { before_id: targetId } : { after_id: targetId }
				)
			});
			if (recordUndo && oldTarget)
				undo = {
					source: sourceId,
					target: oldTarget.id,
					placement: oldPlacement,
					label: source?.name ?? 'channel'
				};
		} catch {
			client.setQueryData(key, previous);
			message = 'The new order could not be saved.';
		}
	}
	async function undoMove() {
		if (!undo) return;
		const action = undo;
		undo = null;
		await move(action.source, action.target, action.placement, false);
	}
	function toggle(id: number) {
		if (selectedGroupManaged) return;
		const next = new Set(selectedIds);
		next.has(id) ? next.delete(id) : next.add(id);
		selectedIds = next;
	}
	async function removeChannel(id: number, label: string) {
		if (selectedGroupManaged) return;
		if (
			!confirm(
				`Remove “${label}”? This deletes the canonical channel and any source matches attached to it.`
			)
		)
			return;
		try {
			await api(`/api/v2/studio/groups/${selectedGroupId}/channels/batch-remove`, {
				method: 'POST',
				body: JSON.stringify({ channel_ids: [id] })
			});
			if (selectedChannelId === id) selectedChannelId = null;
			await refreshWorkspace();
		} catch {
			message = 'The channel could not be removed.';
		}
	}
	async function removeSelected() {
		if (selectedGroupManaged) return;
		if (
			!selectedIds.size ||
			!confirm(`Remove ${selectedIds.size} selected channels? This cannot be undone.`)
		)
			return;
		try {
			await api(`/api/v2/studio/groups/${selectedGroupId}/channels/batch-remove`, {
				method: 'POST',
				body: JSON.stringify({ channel_ids: [...selectedIds] })
			});
			selectedIds = new Set();
			selectedChannelId = null;
			await refreshWorkspace();
		} catch {
			message = 'Some selected channels could not be removed.';
		}
	}
	async function moveSelected() {
		if (selectedGroupManaged || !selectedGroupId || !batchTargetGroupId || !selectedIds.size)
			return;
		try {
			await api(`/api/v2/studio/groups/${selectedGroupId}/channels/batch-move`, {
				method: 'POST',
				body: JSON.stringify({
					channel_ids: [...selectedIds],
					target_group_id: batchTargetGroupId
				})
			});
			const moved = selectedIds.size;
			selectedIds = new Set();
			selectedChannelId = null;
			selectedGroupId = batchTargetGroupId;
			batchTargetGroupId = 0;
			message = `${moved} ${moved === 1 ? 'channel' : 'channels'} moved.`;
			await refreshWorkspace();
		} catch {
			message = 'The selected channels could not be moved.';
		}
	}
	async function addSource(source: SourceChannel) {
		if (!selectedGroupId || selectedGroupManaged) return;
		try {
			await api(`/api/v2/studio/groups/${selectedGroupId}/channels/batch-add`, {
				method: 'POST',
				body: JSON.stringify({ source_channel_ids: [source.id] })
			});
			message = `${source.name} added to the lineup.`;
			await refreshWorkspace();
		} catch {
			message = 'That source channel could not be added.';
		}
	}
	function beginSourceGroupSync(group: SourceGroup) {
		openSyncSettings(null);
		chooseSourceGroup(group);
	}
	async function copySourceGroup(group: SourceGroup) {
		if (sourceGroupAction) return;
		sourceGroupAction = `copy:${group.id}`;
		try {
			const result = await api<SourceGroupImportResult>(
				`/api/v2/studio/lineups/${lineupId}/source-groups/${group.id}/copy`,
				{ method: 'POST' }
			);
			selectedGroupId = result.group_id;
			selectedChannelId = null;
			selectedIds = new Set();
			message = `${result.group_name} created with ${result.added_count} channel${result.added_count === 1 ? '' : 's'}. It is a manual snapshot and will not follow source updates.`;
			await refreshWorkspace();
		} catch (error) {
			message = requestError(error, 'The source group could not be copied.');
		} finally {
			sourceGroupAction = null;
		}
	}
	async function addSourceGroup(group: SourceGroup) {
		const destination = selectedGroup;
		if (!destination || destination.source_link || sourceGroupAction) return;
		if (
			!confirm(
				`Add every missing channel from “${group.name}” to “${destination.name}”? Channels already represented there will be skipped.`
			)
		)
			return;
		sourceGroupAction = `add:${group.id}`;
		try {
			const result = await api<SourceGroupImportResult>(
				`/api/v2/studio/groups/${destination.id}/source-groups/${group.id}/add`,
				{ method: 'POST' }
			);
			message = result.added_count
				? `${result.added_count} channel${result.added_count === 1 ? '' : 's'} added to ${destination.name}${result.skipped_count ? `; ${result.skipped_count} already present.` : '.'}`
				: `Every channel from ${group.name} is already present in ${destination.name}.`;
			await refreshWorkspace();
		} catch (error) {
			message = requestError(error, 'The source group could not be added.');
		} finally {
			sourceGroupAction = null;
		}
	}
	async function attachSource(source: SourceChannel) {
		if (!selectedChannelId) return;
		try {
			await api(`/api/v2/studio/channels/${selectedChannelId}/matches/${source.id}/accept`, {
				method: 'POST'
			});
			message = `${source.name} is now a locked source variant.`;
			await Promise.all([
				client.invalidateQueries({ queryKey: ['studio', 'channel-variants', selectedChannelId] }),
				refreshWorkspace()
			]);
		} catch {
			message = 'The source variant could not be attached.';
		}
	}
	async function rejectSource(source: MatchReview) {
		if (!selectedChannelId) return;
		try {
			await api(
				`/api/v2/studio/channels/${selectedChannelId}/matches/${source.source_channel_id}/reject`,
				{ method: 'POST' }
			);
			message = `${source.source_name} was rejected and will not be automatically rematched.`;
			await Promise.all([
				client.invalidateQueries({ queryKey: ['studio', 'channel-variants', selectedChannelId] }),
				client.invalidateQueries({ queryKey: ['studio', 'channel-rejections', selectedChannelId] }),
				refreshWorkspace()
			]);
		} catch {
			message = 'The source match could not be rejected.';
		}
	}
	async function saveChannel() {
		if (!selectedChannel || !editName.trim()) return;
		saving = true;
		try {
			await api(`/api/v2/studio/channels/${selectedChannel.id}`, {
				method: 'PATCH',
				body: JSON.stringify({
					name: editName.trim(),
					tvg_id: editTvg.trim() || null,
					logo_id: editLogo
				})
			});
			editorId = null;
			await refreshWorkspace();
			message = 'Channel details saved.';
		} catch {
			message = 'Channel details could not be saved.';
		} finally {
			saving = false;
		}
	}
	async function newGroup() {
		const name = prompt('Group name');
		if (!name?.trim()) return;
		try {
			const response = await api<{ group_id: number }>(
				`/api/v2/studio/lineups/${lineupId}/groups`,
				{
					method: 'POST',
					body: JSON.stringify({ name: name.trim() })
				}
			);
			selectedGroupId = response.group_id;
			await refreshWorkspace();
		} catch (error) {
			message = requestError(error, 'The group could not be created.');
		}
	}
	async function renameGroup(group: StudioGroup) {
		if (group.source_link) {
			openSyncSettings(group);
			return;
		}
		const name = prompt('Rename group', group.name);
		if (!name?.trim() || name.trim() === group.name) return;
		try {
			await api(`/api/v2/studio/groups/${group.id}`, {
				method: 'PATCH',
				body: JSON.stringify({ name: name.trim() })
			});
			await refreshWorkspace();
		} catch (error) {
			message = requestError(error, 'The group could not be renamed.');
		}
	}
	async function removeGroup(group: StudioGroup) {
		if (
			!confirm(
				`Delete “${group.name}” and its lineup membership? Channels that are not used elsewhere may also be removed.`
			)
		)
			return;
		try {
			await api(`/api/v2/studio/groups/${group.id}`, { method: 'DELETE' });
			selectedGroupId = null;
			selectedChannelId = null;
			await refreshWorkspace();
		} catch (error) {
			message = requestError(error, 'The group could not be deleted.');
		}
	}
	async function publish() {
		message = 'Starting publication…';
		try {
			await api(`/api/v2/studio/lineups/${lineupId}/publish`, { method: 'POST' });
			message = 'M3U and XMLTV publication is queued.';
		} catch {
			message = 'Publishing could not be started.';
		}
	}
</script>

<svelte:head><title>{lineup?.name ?? 'Lineup'} · Xivi Studio</title></svelte:head>
<div class="workbench">
	<header class="workbench-bar">
		<div>
			<a href="/studio/lineups">Lineups</a><ChevronRight size={14} /><strong
				>{lineup?.name ?? 'Loading…'}</strong
			>
		</div>
		<div class="workbench-actions">
			{#if undo}<button class="app-button app-button--secondary" onclick={undoMove}
					><Undo2 size={17} />Undo “{undo.label}”</button
				>{/if}<button
				class="app-button app-button--secondary"
				onclick={() => (inspectorOpen = !inspectorOpen)}
				><PanelRightClose size={17} />Inspector</button
			><button class="app-button app-button--primary" onclick={publish}
				><UploadCloud size={18} />Publish</button
			>
		</div>
	</header>
	{#if message}<div class="workbench-message" role="status">
			{message}<button onclick={() => (message = '')}>Dismiss</button>
		</div>{/if}
	<div class:inspector-closed={!inspectorOpen} class="workbench-panes">
		<aside class="source-pane">
			<SourceBrowser
				{selectedGroup}
				{selectedChannelId}
				{selectedSourceId}
				actionKey={sourceGroupAction}
				onSelectSource={(source) => (selectedSourceId = source.id)}
				onAddSource={addSource}
				onAttachSource={attachSource}
				onSyncGroup={beginSourceGroupSync}
				onCopyGroup={copySourceGroup}
				onAddGroup={addSourceGroup}
			/>
		</aside>

		<section class="canvas-pane">
			<header>
				<div>
					<p class="eyebrow">Lineup canvas</p>
					<h2>{lineup?.name}</h2>
				</div>
				<div class="group-create-actions">
					<button class="app-button app-button--secondary" onclick={() => openSyncSettings(null)}
						><RadioTower size={17} />Sync group</button
					><button class="app-button app-button--secondary" onclick={newGroup}
						><Plus size={17} />Group</button
					>
				</div>
			</header>
			<div class="group-tabs">
				{#each groupsQuery.data?.items ?? [] as group}<div
						class:active={selectedGroupId === group.id}
					>
						<button
							onclick={() => {
								selectedGroupId = group.id;
								selectedChannelId = null;
								selectedIds = new Set();
							}}
							>{group.name}<span>{group.channel_count}</span>{#if group.source_link}<em
									class:issue={group.source_link.status === 'error' ||
										group.source_link.status === 'disconnected'}
									>{group.source_link.status === 'active' ? 'Synced' : group.source_link.status}</em
								>{/if}</button
						><button
							class="group-menu"
							onclick={() => (group.source_link ? openSyncSettings(group) : renameGroup(group))}
							title={group.source_link ? 'Sync settings' : 'Rename group'}
							>{#if group.source_link}<RadioTower size={14} />{:else}<Pencil
									size={14}
								/>{/if}</button
						><button class="group-menu" onclick={() => removeGroup(group)} title="Delete group"
							><Trash2 size={14} /></button
						>
					</div>{/each}
			</div>
			{#if selectedGroup?.source_link}
				<div
					class="sync-banner"
					class:issue={selectedGroup.source_link.status === 'error' ||
						selectedGroup.source_link.status === 'disconnected'}
				>
					<RadioTower size={18} />
					<span
						><strong>{selectedGroup.source_link.source_group_name}</strong><small
							>{selectedGroup.source_link.playlist_name} · {selectedGroup.source_link.status ===
							'active'
								? 'Source controls membership and order'
								: selectedGroup.source_link.last_error || 'Waiting for its first sync'}</small
						></span
					>
					<button onclick={() => syncNow(selectedGroup)}><RefreshCw size={15} />Sync now</button>
					<button onclick={() => openSyncSettings(selectedGroup)}>Settings</button>
				</div>
			{/if}
			<div class="canvas-tools">
				<label
					><Search size={16} /><input
						bind:value={channelSearch}
						placeholder="Filter lineup"
					/></label
				>
				{#if selectedIds.size}
					<span>{selectedIds.size} selected</span>
					<select bind:value={batchTargetGroupId} aria-label="Move selected channels to group">
						<option value={0} disabled>Move to…</option>
						{#each groupsQuery.data?.items ?? [] as group}
							<option value={group.id}>
								{group.name}{group.id === selectedGroupId ? ' (bottom)' : ''}
							</option>
						{/each}
					</select>
					<button class="batch-move" onclick={moveSelected} disabled={!batchTargetGroupId}
						>Move</button
					>
					<button onclick={removeSelected}><Trash2 size={15} />Remove selected</button>
				{/if}
			</div>
			<div class="channel-list">
				{#if !selectedGroupId}<div class="pane-empty">
						<h3>Create or select a group</h3>
						<p>Groups give your lineup its Watch rails and published order.</p>
					</div>{:else if channelsQuery.isPending}{#each Array(9) as _}<div
							class="channel-skeleton skeleton"
						></div>{/each}{:else if !channelsQuery.data?.total}<div class="pane-empty">
						<h3>This group is ready</h3>
						<p>
							{selectedGroupManaged
								? 'Its connected source group currently has no available channels.'
								: 'Add channels from the source browser. You can reorder them here at any time.'}
						</p>
					</div>{:else}{#each channelsQuery.data.items as channel, index (channel.id)}<WorkspaceRow
							{channel}
							selected={channel.id === selectedChannelId}
							checked={selectedIds.has(channel.id)}
							managed={selectedGroupManaged}
							previousId={channelsQuery.data.items[index - 1]?.id}
							nextId={channelsQuery.data.items[index + 1]?.id}
							firstId={channelsQuery.data.items[0]?.id}
							lastId={channelsQuery.data.items.at(-1)?.id}
							onselect={() => {
								selectedChannelId = channel.id;
								inspectorOpen = true;
							}}
							ontoggle={() => toggle(channel.id)}
							onmove={move}
							onremove={() => removeChannel(channel.id, channel.name)}
						/>{/each}{/if}
			</div>
		</section>

		{#if inspectorOpen}<aside class="inspector-pane">
				<header>
					<p class="eyebrow">Inspector</p>
					<h2>{selectedChannel?.name ?? 'No channel selected'}</h2>
				</header>
				{#if selectedChannel}<form
						onsubmit={(event) => {
							event.preventDefault();
							void saveChannel();
						}}
					>
						<section>
							<h3>Identity</h3>
							{#if selectedGroup?.source_link?.follow_channel_names}<p class="managed-note">
									<RadioTower size={14} />Channel names follow the connected source. Turn this off
									in sync settings to edit names manually.
								</p>{/if}
							<label
								>Channel name<input
									bind:value={editName}
									disabled={selectedGroup?.source_link?.follow_channel_names}
								/></label
							><label>TVG ID<input bind:value={editTvg} placeholder="Schedule ID" /></label>
							<div class="logo-field">
								<span class="field-label">Logo</span>
								<button
									type="button"
									class="logo-picker-trigger"
									aria-expanded={logoPickerOpen}
									onclick={() => (logoPickerOpen = !logoPickerOpen)}
								>
									{#key editLogo}<LogoTile
											src={logoSrc(selectedLogo?.image)}
											name={selectedLogo?.name ?? 'Signal Tile'}
											size="sm"
											contrast
										/>{/key}
									<span class="logo-picker-copy">
										<strong>{selectedLogo?.name ?? 'Signal Tile'}</strong>
										<small>Browse {(logosQuery.data?.logos ?? []).length} logo assets</small>
									</span>
									<ChevronDown size={17} class={logoPickerOpen ? 'open' : ''} />
								</button>
								{#if logoPickerOpen}<div class="logo-picker-panel">
										<label class="logo-search">
											<Search size={15} />
											<span class="sr-only">Search logos</span>
											<input
												bind:value={logoSearch}
												placeholder="Search logos"
												onkeydown={(event) => {
													if (event.key === 'Enter') event.preventDefault();
												}}
											/>
										</label>
										{#if logosQuery.isPending}<div
												class="logo-loading skeleton"
											></div>{:else if visibleLogos.length}<div class="logo-options">
												{#each visibleLogos as logo}<button
														type="button"
														class:selected={logo.id === editLogo}
														aria-pressed={logo.id === editLogo}
														aria-label={`Select ${logo.name} logo`}
														onclick={() => {
															editLogo = logo.id;
															logoPickerOpen = false;
														}}
													>
														<LogoTile
															src={logoSrc(logo.image)}
															name={logo.name}
															size="md"
															contrast
														/>
														<span>{logo.name}</span>
														{#if logo.id === editLogo}<Check size={15} />{/if}
													</button>{/each}
											</div>{:else}<p class="logo-empty">No logos match “{logoSearch}”.</p>{/if}
										{#if matchingLogos.length > visibleLogos.length}<p class="logo-results">
												Showing the first {visibleLogos.length} results. Refine your search to see more.
											</p>{/if}
									</div>{/if}
							</div>
							<button class="app-button app-button--primary" disabled={saving || !editName.trim()}
								>{saving ? 'Saving…' : 'Save identity'}</button
							>
						</section>
					</form>
					<section>
						<h3>Match health</h3>
						<dl class="match-details">
							<div>
								<dt>Method</dt>
								<dd>{selectedChannel.match_method || 'Unmatched'}</dd>
							</div>
							<div>
								<dt>Confidence</dt>
								<dd>
									{selectedChannel.match_score !== undefined
										? `${Math.round(selectedChannel.match_score * 100)}%`
										: '—'}
								</dd>
							</div>
							<div>
								<dt>Runner-up</dt>
								<dd>
									{selectedChannel.runner_up_score !== undefined
										? `${Math.round(selectedChannel.runner_up_score * 100)}%`
										: '—'}
								</dd>
							</div>
							<div>
								<dt>Lock</dt>
								<dd>
									{#if selectedChannel.manual_locked}<LockKeyhole size={14} /> Locked{:else}Automatic{/if}
								</dd>
							</div>
						</dl>
					</section>
					<section>
						<h3>Ordered source variants</h3>
						{#if variantsQuery.isPending}<div
								class="source-skeleton skeleton"
							></div>{:else if variantsQuery.data?.items.length}<ol class="variants">
								{#each variantsQuery.data.items as variant}<li>
										<CircleGauge size={16} /><span
											><strong>{variant.source_name}</strong><small
												>{variant.playlist_name} · {variant.method}{variant.score !== undefined
													? ` · ${Math.round(variant.score * 100)}%`
													: ''}</small
											></span
										><button
											type="button"
											onclick={() => rejectSource(variant)}
											aria-label={`Reject ${variant.source_name}`}><Unlink size={15} /></button
										>
									</li>{/each}
							</ol>{:else}<p class="muted small">
								No source variants. Choose “Variant” in the source browser to add one and lock the
								match.
							</p>{/if}
					</section>
					{#if rejectionsQuery.data?.items.length}<section>
							<h3><History size={15} /> Rejection history</h3>
							<ul class="rejections">
								{#each rejectionsQuery.data.items as rejection}<li>
										<strong>{rejection.name || rejection.tvg_id}</strong><small
											>{rejection.playlist_name} · {new Date(
												rejection.created_at
											).toLocaleDateString()}</small
										>
									</li>{/each}
							</ul>
						</section>{/if}
					<section class="danger-zone">
						<h3>Destructive actions</h3>
						<button
							type="button"
							disabled={selectedGroupManaged}
							onclick={() => removeChannel(selectedChannel!.id, selectedChannel!.name)}
							><Trash2 size={16} />Delete channel</button
						>
					</section>{:else}<div class="pane-empty">
						<MoreHorizontal size={28} />
						<h3>Select a channel</h3>
						<p>
							Identity, logo, guide mapping, confidence, source variants, and destructive actions
							appear here.
						</p>
					</div>{/if}
			</aside>{/if}
	</div>
</div>

<Dialog.Root bind:open={syncDialogOpen}>
	<Dialog.Portal>
		<Dialog.Overlay class="sync-overlay" />
		<Dialog.Content class="sync-dialog" aria-describedby="sync-description">
			<header class="sync-dialog-header">
				<div>
					<p class="eyebrow">One-way source subscription</p>
					<Dialog.Title class="sync-title"
						>{syncGroup ? `Sync settings · ${syncGroup.name}` : 'Create synced group'}</Dialog.Title
					>
				</div>
				<Dialog.Close class="sync-close" aria-label="Close sync settings"
					><X size={20} /></Dialog.Close
				>
			</header>
			<Dialog.Description id="sync-description" class="sync-description">
				Membership, order, and stream associations follow the selected source group. Guide and logo
				enrichment remain yours.
			</Dialog.Description>
			{#if syncGroup?.source_link}
				<div
					class="sync-current"
					class:issue={syncGroup.source_link.status === 'error' ||
						syncGroup.source_link.status === 'disconnected'}
				>
					{#if syncGroup.source_link.status === 'error' || syncGroup.source_link.status === 'disconnected'}
						<CircleAlert size={18} />
					{:else}
						<RadioTower size={18} />
					{/if}
					<span
						><strong>{syncGroup.source_link.status}</strong><small
							>{syncGroup.source_link.last_error ||
								(syncGroup.source_link.last_synced_at
									? `Last synced ${new Date(syncGroup.source_link.last_synced_at).toLocaleString()}`
									: 'Waiting for its first sync')}</small
						></span
					>
				</div>
			{/if}
			<div class="sync-dialog-body">
				{#if syncError}<div class="sync-error" role="alert">
						<CircleAlert size={18} />{syncError}
					</div>{/if}
				{#if !syncGroup}<label class="sync-group-name">
						<span>Lineup group name</span>
						<input
							bind:value={syncGroupName}
							oninput={() => (syncError = '')}
							maxlength="255"
							placeholder="Group name"
						/>
					</label>{/if}
				<label class="sync-search">
					<Search size={16} /><span class="sr-only">Search source groups</span><input
						bind:value={syncSearch}
						placeholder="Search source groups"
					/>
				</label>
				<div class="source-group-options" role="radiogroup" aria-label="Source group">
					{#if sourceGroupsQuery.isPending}
						{#each Array(5) as _}<div class="source-group-skeleton skeleton"></div>{/each}
					{:else if matchingSourceGroups.length}
						{#each matchingSourceGroups as group}
							<button
								type="button"
								role="radio"
								aria-checked={syncSourceGroupId === group.id}
								class:selected={syncSourceGroupId === group.id}
								disabled={!group.enabled || group.channel_count === 0}
								onclick={() => chooseSourceGroup(group)}
							>
								<span
									><strong>{group.name}</strong><small
										>{group.playlist_name} · {group.channel_count} channel{group.channel_count === 1
											? ''
											: 's'}{group.linked_group_count
											? ` · ${group.linked_group_count} existing link${group.linked_group_count === 1 ? '' : 's'}`
											: ''}</small
									></span
								>{#if syncSourceGroupId === group.id}<Check size={17} />{/if}
							</button>
						{/each}
					{:else}
						<p class="sync-empty">No source groups match this search.</p>
					{/if}
				</div>
				<div class="sync-policies">
					<label
						><input type="checkbox" bind:checked={followGroupName} /><span
							><strong>Follow source group name</strong><small
								>Rename this lineup group when the provider renames its group.</small
							></span
						></label
					>
					<label
						><input type="checkbox" bind:checked={followChannelNames} /><span
							><strong>Follow source channel names</strong><small
								>Names update automatically; TVG and logo enrichment stay unchanged.</small
							></span
						></label
					>
				</div>
			</div>
			<footer class="sync-dialog-footer">
				{#if syncGroup?.source_link}<div class="disconnect-actions">
						<button disabled={syncSaving} onclick={() => disconnectSync(true)}
							>Disconnect & keep</button
						>
						<button class="danger" disabled={syncSaving} onclick={() => disconnectSync(false)}
							>Disconnect & remove</button
						>
					</div>{/if}
				<span></span>
				<Dialog.Close class="app-button app-button--secondary">Cancel</Dialog.Close>
				<button
					class="app-button app-button--primary"
					disabled={syncSaving || !syncSourceGroupId || (!syncGroup && !syncGroupName.trim())}
					onclick={saveSyncSettings}
					>{syncSaving ? 'Saving…' : syncGroup ? 'Save & sync' : 'Create & sync'}</button
				>
			</footer>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.workbench {
		height: 100dvh;
		overflow: hidden;
	}
	.workbench-bar {
		display: flex;
		height: 4.4rem;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		background: var(--surface);
		padding: 0 1rem;
	}
	.workbench-bar > div {
		display: flex;
		align-items: center;
		gap: 0.45rem;
	}
	.workbench-bar a {
		color: var(--muted);
		font-size: 0.75rem;
	}
	.workbench-actions {
		gap: 0.4rem !important;
	}
	.workbench-message {
		display: flex;
		height: 2.7rem;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		background: var(--sun);
		padding: 0.4rem;
		color: var(--ink);
		font-size: 0.75rem;
		font-weight: 750;
	}
	.workbench-message button {
		border: 0;
		background: transparent;
		text-decoration: underline;
		cursor: pointer;
	}
	.workbench-panes {
		display: grid;
		height: calc(100dvh - 4.4rem);
		grid-template-columns: minmax(15rem, 20rem) minmax(23rem, 1fr) minmax(17rem, 21rem);
	}
	.workbench-message + .workbench-panes {
		height: calc(100dvh - 7.1rem);
	}
	.workbench-panes.inspector-closed {
		grid-template-columns: minmax(16rem, 22rem) 1fr;
	}
	.source-pane,
	.canvas-pane,
	.inspector-pane {
		display: flex;
		min-width: 0;
		overflow: hidden;
		flex-direction: column;
		background: var(--surface);
	}
	.canvas-pane {
		border-inline: 1px solid var(--line);
		background: var(--deep);
	}
	.canvas-pane > header,
	.inspector-pane > header {
		display: flex;
		min-height: 4.2rem;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 0.7rem 1rem;
	}
	.canvas-pane h2,
	.inspector-pane h2 {
		overflow: hidden;
		margin: 0;
		font-size: 1.2rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.canvas-tools > label {
		display: flex;
		min-height: 2.7rem;
		align-items: center;
		gap: 0.45rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface-raised);
		padding: 0 0.65rem;
		color: var(--muted);
	}
	.canvas-tools input {
		min-width: 0;
		width: 100%;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
		font-size: 0.75rem;
	}
	.channel-list,
	.inspector-pane {
		overflow-y: auto;
	}
	.group-create-actions {
		display: flex;
		gap: 0.4rem;
	}
	.group-tabs {
		display: flex;
		min-height: 3.2rem;
		gap: 0.3rem;
		overflow-x: auto;
		border-bottom: 1px solid var(--line);
		background: var(--surface);
		padding: 0.45rem;
	}
	.group-tabs > div {
		display: flex;
		border: 1px solid transparent;
		border-radius: 0.65rem;
	}
	.group-tabs > div.active {
		border-color: color-mix(in oklch, var(--periwinkle) 65%, transparent);
		background: color-mix(in oklch, var(--periwinkle) 13%, transparent);
	}
	.group-tabs button {
		display: flex;
		min-height: 2.3rem;
		align-items: center;
		gap: 0.35rem;
		white-space: nowrap;
		border: 0;
		background: transparent;
		padding: 0.35rem 0.5rem;
		color: var(--muted);
		font-size: 0.68rem;
		font-weight: 750;
		cursor: pointer;
	}
	.group-tabs span {
		border-radius: 99px;
		background: var(--surface-raised);
		padding: 0.1rem 0.35rem;
	}
	.group-tabs em {
		color: var(--aqua);
		font-size: 0.55rem;
		font-style: normal;
		text-transform: uppercase;
	}
	.group-tabs em.issue {
		color: var(--error);
	}
	.group-tabs .group-menu {
		display: none;
		width: 1.8rem;
		padding: 0;
	}
	.group-tabs > div:hover .group-menu,
	.group-tabs > div.active .group-menu {
		display: grid;
		place-items: center;
	}
	.sync-banner {
		display: grid;
		min-height: 3.4rem;
		grid-template-columns: auto minmax(0, 1fr) auto auto;
		align-items: center;
		gap: 0.65rem;
		border-bottom: 1px solid color-mix(in oklch, var(--aqua) 35%, var(--line));
		background: color-mix(in oklch, var(--aqua) 10%, var(--surface));
		padding: 0.55rem 0.7rem;
		color: var(--aqua);
	}
	.sync-banner.issue {
		border-color: color-mix(in oklch, var(--error) 45%, var(--line));
		background: color-mix(in oklch, var(--error) 9%, var(--surface));
		color: var(--error);
	}
	.sync-banner > span {
		display: grid;
		min-width: 0;
		color: var(--text);
	}
	.sync-banner strong,
	.sync-banner small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.sync-banner strong {
		font-size: 0.7rem;
	}
	.sync-banner small {
		color: var(--muted);
		font-size: 0.58rem;
	}
	.sync-banner button {
		display: flex;
		min-height: 2.25rem;
		align-items: center;
		gap: 0.3rem;
		border: 1px solid var(--line);
		border-radius: 0.55rem;
		background: var(--surface-raised);
		padding: 0 0.55rem;
		color: var(--text);
		font-size: 0.62rem;
		font-weight: 750;
		cursor: pointer;
	}
	.canvas-tools {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		border-bottom: 1px solid var(--line);
		background: var(--surface);
		padding: 0.5rem;
	}
	.canvas-tools label {
		max-width: 18rem;
		flex: 1;
	}
	.canvas-tools > span {
		margin-left: auto;
		color: var(--muted);
		font-size: 0.68rem;
	}
	.canvas-tools > select {
		min-height: 2.3rem;
		max-width: 9rem;
		border: 1px solid var(--line);
		border-radius: 0.6rem;
		background: var(--surface-raised);
		padding: 0 0.45rem;
		color: var(--text);
		font: inherit;
		font-size: 0.65rem;
	}
	.canvas-tools > button {
		display: flex;
		min-height: 2.3rem;
		align-items: center;
		gap: 0.3rem;
		border: 1px solid color-mix(in oklch, var(--error) 40%, var(--line));
		border-radius: 0.6rem;
		background: transparent;
		color: var(--error);
		padding: 0.35rem 0.55rem;
		font-size: 0.65rem;
		cursor: pointer;
	}
	.canvas-tools > button.batch-move {
		border-color: color-mix(in oklch, var(--aqua) 45%, var(--line));
		color: var(--text);
	}
	.channel-list {
		flex: 1;
	}
	.channel-skeleton {
		height: 4rem;
		border-bottom: 1px solid var(--line);
	}
	.source-skeleton {
		height: 3.6rem;
		margin: 0.45rem;
		border-radius: 0.6rem;
	}
	.pane-empty {
		display: grid;
		min-height: 16rem;
		place-items: center;
		align-content: center;
		padding: 2rem;
		text-align: center;
		color: var(--muted);
	}
	.pane-empty h3 {
		margin: 0.5rem 0 0;
	}
	.pane-empty p {
		max-width: 20rem;
		margin: 0.3rem 0;
		font-size: 0.75rem;
	}
	.inspector-pane > form > section,
	.inspector-pane > section {
		display: grid;
		width: 100%;
		min-width: 0;
		gap: 0.65rem;
		border-bottom: 1px solid var(--line);
		padding: 1rem;
	}
	.inspector-pane section h3 {
		margin: 0;
		font-size: 0.82rem;
	}
	.inspector-pane label {
		display: grid;
		min-width: 0;
		gap: 0.3rem;
		color: var(--muted);
		font-size: 0.65rem;
	}
	.inspector-pane input {
		width: 100%;
		min-width: 0;
		max-width: 100%;
		min-height: 2.6rem;
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		background: var(--surface-raised);
		padding: 0.5rem 0.65rem;
		color: var(--text);
	}
	.inspector-pane input:disabled {
		opacity: 0.62;
		cursor: not-allowed;
	}
	.managed-note {
		display: flex;
		align-items: flex-start;
		gap: 0.45rem;
		margin: 0;
		border-radius: 0.6rem;
		background: color-mix(in oklch, var(--aqua) 10%, var(--surface-raised));
		padding: 0.55rem;
		color: var(--muted);
		font-size: 0.62rem;
		line-height: 1.45;
	}
	.managed-note :global(svg) {
		flex: none;
		color: var(--aqua);
	}
	.inspector-pane > form {
		width: 100%;
		min-width: 0;
	}
	.inspector-pane > form .app-button {
		width: 100%;
		min-width: 0;
	}
	.logo-field {
		display: grid;
		min-width: 0;
		gap: 0.3rem;
	}
	.field-label {
		color: var(--muted);
		font-size: 0.65rem;
	}
	.logo-picker-trigger {
		display: grid;
		width: 100%;
		min-width: 0;
		min-height: 3.45rem;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.65rem;
		border: 1px solid var(--line);
		border-radius: 0.75rem;
		background: var(--surface-raised);
		padding: 0.5rem;
		text-align: left;
		cursor: pointer;
	}
	.logo-picker-trigger:hover,
	.logo-picker-trigger[aria-expanded='true'] {
		border-color: color-mix(in oklch, var(--aqua) 55%, var(--line));
		background: color-mix(in oklch, var(--aqua) 7%, var(--surface-raised));
	}
	.logo-picker-trigger > :global(svg) {
		color: var(--muted);
		transition: transform var(--layout) var(--ease-out);
	}
	.logo-picker-trigger > :global(svg.open) {
		transform: rotate(180deg);
	}
	.logo-picker-copy {
		display: grid;
		min-width: 0;
	}
	.logo-picker-copy strong,
	.logo-picker-copy small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.logo-picker-copy strong {
		font-size: 0.74rem;
	}
	.logo-picker-copy small {
		color: var(--muted);
		font-size: 0.58rem;
	}
	.logo-picker-panel {
		display: grid;
		min-width: 0;
		gap: 0.55rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--deep);
		padding: 0.55rem;
	}
	.logo-search {
		display: flex !important;
		min-height: 2.5rem;
		align-items: center;
		gap: 0.45rem !important;
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		background: var(--surface);
		padding: 0 0.6rem;
		color: var(--muted) !important;
	}
	.logo-search input {
		min-height: 0;
		border: 0;
		outline: 0;
		background: transparent;
		padding: 0;
	}
	.logo-options {
		display: grid;
		max-height: 18rem;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.4rem;
		overflow-y: auto;
		padding-right: 0.15rem;
	}
	.logo-options button {
		position: relative;
		display: grid;
		min-width: 0;
		min-height: 5.6rem;
		place-items: center;
		align-content: center;
		gap: 0.3rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface);
		padding: 0.45rem;
		cursor: pointer;
	}
	.logo-options button:hover,
	.logo-options button.selected {
		border-color: var(--aqua);
		background: color-mix(in oklch, var(--aqua) 10%, var(--surface));
	}
	.logo-options button > span:not(:global(.logo-tile)) {
		width: 100%;
		overflow: hidden;
		font-size: 0.61rem;
		font-weight: 750;
		text-align: center;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.logo-options button > :global(svg) {
		position: absolute;
		top: 0.35rem;
		right: 0.35rem;
		border-radius: 50%;
		background: var(--aqua);
		padding: 0.12rem;
		color: var(--ink);
	}
	.logo-loading {
		height: 8rem;
		border-radius: 0.65rem;
	}
	.logo-empty,
	.logo-results {
		margin: 0;
		color: var(--muted);
		font-size: 0.6rem;
		text-align: center;
	}
	.match-details {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.55rem;
		margin: 0;
	}
	.match-details div {
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		padding: 0.5rem;
	}
	.match-details dt {
		color: var(--muted);
		font-size: 0.6rem;
	}
	.match-details dd {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		margin: 0.1rem 0 0;
		font-size: 0.72rem;
		font-weight: 750;
	}
	.variants {
		display: grid;
		gap: 0.35rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.variants li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		border: 1px solid var(--line);
		border-radius: 0.6rem;
		padding: 0.5rem;
	}
	.variants li span {
		display: grid;
		min-width: 0;
		flex: 1;
	}
	.variants li > button {
		display: grid;
		width: 2rem;
		height: 2rem;
		flex: none;
		place-items: center;
		border: 0;
		border-radius: 0.5rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.variants li > button:hover {
		background: var(--surface-raised);
		color: var(--error);
	}
	.variants strong {
		font-size: 0.68rem;
	}
	.variants small,
	.small {
		font-size: 0.6rem;
	}
	.inspector-pane h3 {
		display: flex;
		align-items: center;
		gap: 0.35rem;
	}
	.rejections {
		display: grid;
		gap: 0.4rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.rejections li {
		display: grid;
		border-left: 2px solid var(--error);
		padding-left: 0.55rem;
	}
	.rejections strong {
		font-size: 0.66rem;
	}
	.rejections small {
		color: var(--muted);
		font-size: 0.58rem;
	}
	.danger-zone button {
		display: flex;
		min-height: 2.6rem;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		border: 1px solid color-mix(in oklch, var(--error) 45%, var(--line));
		border-radius: 0.65rem;
		background: transparent;
		color: var(--error);
		cursor: pointer;
	}
	.danger-zone button:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	:global(.sync-overlay) {
		position: fixed;
		z-index: 90;
		inset: 0;
		background: rgb(8 10 15 / 0.74);
	}
	:global(.sync-dialog) {
		position: fixed;
		z-index: 91;
		top: 50%;
		left: 50%;
		display: flex;
		width: min(42rem, calc(100vw - 2rem));
		max-height: min(48rem, calc(100dvh - 2rem));
		transform: translate(-50%, -50%);
		overflow: hidden;
		flex-direction: column;
		border: 1px solid var(--line);
		border-radius: 1.15rem;
		outline: none;
		background: var(--surface-raised);
		box-shadow: 0 28px 90px rgb(0 0 0 / 0.46);
		color: var(--text);
	}
	.sync-dialog-header,
	.sync-dialog-footer {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		border-bottom: 1px solid var(--line);
		padding: 0.85rem 1rem;
	}
	.sync-dialog-header {
		justify-content: space-between;
	}
	.sync-dialog-footer {
		border-top: 1px solid var(--line);
		border-bottom: 0;
	}
	.sync-dialog-footer > span {
		flex: 1;
	}
	:global(.sync-title) {
		margin: 0;
		font-size: 1.25rem;
	}
	:global(.sync-close) {
		display: grid;
		width: 2.5rem;
		height: 2.5rem;
		flex: none;
		place-items: center;
		border: 0;
		border-radius: 0.65rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	:global(.sync-description) {
		margin: 0;
		padding: 0.8rem 1rem 0;
		color: var(--muted);
		font-size: 0.72rem;
		line-height: 1.5;
	}
	.sync-current {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		margin: 0.75rem 1rem 0;
		border-radius: 0.7rem;
		background: color-mix(in oklch, var(--aqua) 10%, var(--surface));
		padding: 0.65rem;
		color: var(--aqua);
	}
	.sync-current.issue {
		background: color-mix(in oklch, var(--error) 10%, var(--surface));
		color: var(--error);
	}
	.sync-current span {
		display: grid;
	}
	.sync-current strong {
		font-size: 0.7rem;
		text-transform: capitalize;
	}
	.sync-current small {
		color: var(--muted);
		font-size: 0.62rem;
	}
	.sync-dialog-body {
		display: grid;
		min-height: 0;
		gap: 0.7rem;
		overflow-y: auto;
		padding: 0.85rem 1rem 1rem;
	}
	.sync-error {
		display: flex;
		align-items: flex-start;
		gap: 0.55rem;
		border: 1px solid color-mix(in oklch, var(--error) 40%, var(--line));
		border-radius: 0.75rem;
		background: color-mix(in oklch, var(--error) 10%, var(--surface));
		padding: 0.7rem 0.8rem;
		color: var(--error);
		font-size: 0.78rem;
		font-weight: 700;
	}
	.sync-error :global(svg) {
		flex: 0 0 auto;
	}
	.sync-group-name {
		display: grid;
		gap: 0.35rem;
		color: var(--muted);
		font-size: 0.65rem;
		font-weight: 700;
	}
	.sync-group-name input,
	.sync-search {
		min-height: 2.7rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface);
		color: var(--text);
	}
	.sync-group-name input {
		padding: 0 0.7rem;
	}
	.sync-search {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0 0.7rem;
		color: var(--muted);
	}
	.sync-search input {
		min-width: 0;
		flex: 1;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
	}
	.source-group-options {
		display: grid;
		max-height: 17rem;
		gap: 0.35rem;
		overflow-y: auto;
		padding-right: 0.15rem;
	}
	.source-group-options button {
		display: flex;
		min-height: 3.5rem;
		align-items: center;
		gap: 0.65rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface);
		padding: 0.6rem 0.75rem;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}
	.source-group-options button.selected {
		border-color: var(--aqua);
		background: color-mix(in oklch, var(--aqua) 9%, var(--surface));
	}
	.source-group-options button:disabled {
		opacity: 0.42;
		cursor: not-allowed;
	}
	.source-group-options button span {
		display: grid;
		min-width: 0;
		flex: 1;
	}
	.source-group-options button strong,
	.source-group-options button small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.source-group-options button strong {
		font-size: 0.72rem;
	}
	.source-group-options button small {
		color: var(--muted);
		font-size: 0.61rem;
	}
	.source-group-skeleton {
		height: 3.5rem;
		border-radius: 0.7rem;
	}
	.sync-empty {
		margin: 0;
		padding: 1.5rem;
		color: var(--muted);
		font-size: 0.7rem;
		text-align: center;
	}
	.sync-policies {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.45rem;
	}
	.sync-policies label {
		display: flex;
		align-items: flex-start;
		gap: 0.55rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface);
		padding: 0.65rem;
		cursor: pointer;
	}
	.sync-policies input {
		width: 1rem;
		height: 1rem;
		flex: none;
		accent-color: var(--aqua);
	}
	.sync-policies span {
		display: grid;
		gap: 0.15rem;
	}
	.sync-policies strong {
		font-size: 0.66rem;
	}
	.sync-policies small {
		color: var(--muted);
		font-size: 0.58rem;
		line-height: 1.4;
	}
	.disconnect-actions {
		display: flex;
		gap: 0.3rem;
	}
	.disconnect-actions button {
		min-height: 2.5rem;
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		background: transparent;
		padding: 0 0.6rem;
		color: var(--muted);
		font-size: 0.62rem;
		font-weight: 700;
		cursor: pointer;
	}
	.disconnect-actions button.danger {
		border-color: color-mix(in oklch, var(--error) 40%, var(--line));
		color: var(--error);
	}
	@media (max-width: 1150px) {
		.workbench-panes {
			grid-template-columns: 18rem 1fr;
		}
		.inspector-pane {
			position: fixed;
			z-index: 55;
			top: 4.4rem;
			right: 0;
			bottom: 0;
			width: min(23rem, 90vw);
			border-left: 1px solid var(--line);
			box-shadow: -20px 0 60px rgb(0 0 0/0.3);
		}
		.workbench-message + .workbench-panes .inspector-pane {
			top: 7.1rem;
		}
	}
	@media (max-width: 760px) {
		.workbench-bar {
			height: auto;
			min-height: 4rem;
			align-items: flex-start;
			flex-direction: column;
			padding: 0.55rem;
		}
		.workbench-actions {
			width: 100%;
		}
		.workbench-actions button {
			flex: 1;
			font-size: 0.65rem;
		}
		.workbench-panes,
		.workbench-panes.inspector-closed {
			height: calc(100dvh - 7.5rem);
			grid-template-columns: 1fr;
		}
		.source-pane {
			display: none;
		}
		.canvas-pane {
			border: 0;
		}
		.inspector-pane {
			top: 7.5rem;
		}
		.workbench-message + .workbench-panes {
			height: calc(100dvh - 10.2rem);
		}
		.group-create-actions .app-button {
			min-width: 0;
			padding-inline: 0.55rem;
			font-size: 0.62rem;
		}
		.sync-banner {
			grid-template-columns: auto minmax(0, 1fr) auto;
		}
		.sync-banner button:last-child {
			display: none;
		}
		:global(.sync-dialog) {
			width: calc(100vw - 1rem);
			max-height: calc(100dvh - 1rem);
		}
		.sync-policies {
			grid-template-columns: 1fr;
		}
		.sync-dialog-footer {
			align-items: stretch;
			flex-wrap: wrap;
		}
		.sync-dialog-footer > span {
			display: none;
		}
		.disconnect-actions {
			width: 100%;
		}
		.disconnect-actions button {
			flex: 1;
		}
	}
</style>
