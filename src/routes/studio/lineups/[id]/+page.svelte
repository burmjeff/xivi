<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import {
		Search,
		Plus,
		RefreshCw,
		UploadCloud,
		Trash2,
		Undo2,
		ChevronRight,
		PanelRightClose,
		Link2,
		LockKeyhole,
		CircleGauge,
		History,
		Unlink,
		MoreHorizontal,
		Pencil,
		ChevronDown,
		Check
	} from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type {
		LineupSummary,
		MatchRejection,
		MatchReview,
		Paginated,
		SourceChannel,
		StudioGroup,
		WorkspaceChannel
	} from '$lib/api/types';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import WorkspaceRow from '$lib/components/studio/WorkspaceRow.svelte';

	const lineupId = Number(page.params.id),
		client = useQueryClient();
	const lineupsQuery = createQuery(() => ({
		queryKey: ['studio', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/studio/lineups')
	}));
	const groupsQuery = createQuery(() => ({
		queryKey: ['studio', 'lineup-groups', lineupId],
		queryFn: () => api<Paginated<StudioGroup>>(`/api/v2/studio/lineups/${lineupId}/groups`)
	}));
	let lineup = $derived(lineupsQuery.data?.items.find((item) => item.id === lineupId));
	let selectedGroupId = $state<number | null>(Number(page.url.searchParams.get('group')) || null),
		selectedChannelId = $state<number | null>(null),
		selectedSourceId = $state<number | null>(null),
		channelSearch = $state(''),
		sourceSearch = $state(''),
		inspectorOpen = $state(true),
		selectedIds = $state(new Set<number>()),
		batchTargetGroupId = $state(0),
		message = $state(''),
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
	const sourcesQuery = createQuery(() => ({
		queryKey: ['studio', 'source-browser', sourceSearch],
		queryFn: () =>
			api<Paginated<SourceChannel>>(
				`/api/v2/studio/source-channels${params({ q: sourceSearch, limit: 150 })}`
			)
	}));
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

	async function refreshWorkspace() {
		await Promise.all([
			client.invalidateQueries({ queryKey: ['studio', 'lineup-groups', lineupId] }),
			client.invalidateQueries({ queryKey: ['studio', 'group-channels'] }),
			client.invalidateQueries({ queryKey: ['studio', 'source-browser'] })
		]);
	}
	async function move(
		sourceId: number,
		targetId: number,
		placement: 'before' | 'after',
		recordUndo = true
	) {
		if (!selectedGroupId || sourceId === targetId) return;
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
		const next = new Set(selectedIds);
		next.has(id) ? next.delete(id) : next.add(id);
		selectedIds = next;
	}
	async function removeChannel(id: number, label: string) {
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
		if (!selectedGroupId || !batchTargetGroupId || !selectedIds.size) return;
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
		if (!selectedGroupId) return;
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
			const response = await api<{ templategroup: StudioGroup }>('/api/template/group', {
				method: 'POST',
				body: JSON.stringify({ name: name.trim(), dynamic: false, dynamicgroup: null })
			});
			await api(`/api/template/${lineupId}/group/${response.templategroup.id}/item`, {
				method: 'POST'
			});
			selectedGroupId = response.templategroup.id;
			await refreshWorkspace();
		} catch {
			message = 'The group could not be created.';
		}
	}
	async function renameGroup(group: StudioGroup) {
		const name = prompt('Rename group', group.name);
		if (!name?.trim() || name.trim() === group.name) return;
		try {
			await api('/api/template/group', {
				method: 'PUT',
				body: JSON.stringify({
					id: group.id,
					name: name.trim(),
					dynamic: group.dynamic,
					dynamicgroup: group.dynamic_group_id ?? null
				})
			});
			await refreshWorkspace();
		} catch {
			message = 'The group could not be renamed.';
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
			await api(`/api/template/group/${group.id}`, { method: 'DELETE' });
			selectedGroupId = null;
			selectedChannelId = null;
			await refreshWorkspace();
		} catch {
			message = 'The group could not be deleted.';
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
			<header>
				<div>
					<p class="eyebrow">Source browser</p>
					<h2>Available channels</h2>
				</div>
				<button onclick={() => refreshWorkspace()} aria-label="Refresh browser"
					><RefreshCw size={17} /></button
				>
			</header>
			<label class="pane-search"
				><Search size={17} /><input bind:value={sourceSearch} placeholder="Search sources" /></label
			>
			<div class="source-list">
				{#if sourcesQuery.isPending}{#each Array(8) as _}<div
							class="source-skeleton skeleton"
						></div>{/each}{:else}{#each sourcesQuery.data?.items ?? [] as source}<article
							class:selected={source.id === selectedSourceId}
						>
							<button class="source-select" onclick={() => (selectedSourceId = source.id)}>
								<LogoTile src={source.logo_url} name={source.name} size="sm" contrast />
								<span class="source-copy">
									<strong>{source.name}</strong><small
										>{source.playlist_name} · {source.group_name}</small
									>
								</span>
								<span class:off={!source.enabled}>{source.enabled ? 'On' : 'Off'}</span>
							</button>
							<div class="source-actions">
								<button
									onclick={(event) => {
										event.stopPropagation();
										addSource(source);
									}}
									title="Add as a new lineup channel"><Plus size={15} />Add</button
								>{#if selectedChannelId}<button
										onclick={(event) => {
											event.stopPropagation();
											attachSource(source);
										}}
										title="Attach to selected lineup channel"><Link2 size={15} />Variant</button
									>{/if}
							</div>
						</article>{/each}{/if}
			</div>
		</aside>

		<section class="canvas-pane">
			<header>
				<div>
					<p class="eyebrow">Lineup canvas</p>
					<h2>{lineup?.name}</h2>
				</div>
				<button class="app-button app-button--secondary" onclick={newGroup}
					><Plus size={17} />Group</button
				>
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
							>{group.name}<span>{group.channel_count}</span>{#if group.dynamic}<em>Dynamic</em
								>{/if}</button
						><button class="group-menu" onclick={() => renameGroup(group)} title="Rename group"
							><Pencil size={14} /></button
						><button class="group-menu" onclick={() => removeGroup(group)} title="Delete group"
							><Trash2 size={14} /></button
						>
					</div>{/each}
			</div>
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
						<p>Add channels from the source browser. You can reorder them here at any time.</p>
					</div>{:else}{#each channelsQuery.data.items as channel, index (channel.id)}<WorkspaceRow
							{channel}
							selected={channel.id === selectedChannelId}
							checked={selectedIds.has(channel.id)}
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
							<label>Channel name<input bind:value={editName} /></label><label
								>TVG ID<input bind:value={editTvg} placeholder="Schedule ID" /></label
							>
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
	.source-pane > header,
	.canvas-pane > header,
	.inspector-pane > header {
		display: flex;
		min-height: 4.2rem;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 0.7rem 1rem;
	}
	.source-pane h2,
	.canvas-pane h2,
	.inspector-pane h2 {
		overflow: hidden;
		margin: 0;
		font-size: 1.2rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.source-pane > header button {
		display: grid;
		width: 2.4rem;
		height: 2.4rem;
		place-items: center;
		border: 0;
		border-radius: 0.7rem;
		background: var(--surface-raised);
		cursor: pointer;
	}
	.pane-search,
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
	.pane-search {
		margin: 0.65rem;
	}
	.pane-search input,
	.canvas-tools input {
		min-width: 0;
		width: 100%;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
		font-size: 0.75rem;
	}
	.source-list,
	.channel-list,
	.inspector-pane {
		overflow-y: auto;
	}
	.source-list article {
		border-bottom: 1px solid var(--line);
		padding: 0.55rem;
	}
	.source-list article:hover,
	.source-list article.selected {
		background: color-mix(in oklch, var(--aqua) 9%, var(--surface));
	}
	.source-select {
		display: grid;
		width: 100%;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.55rem;
		border: 0;
		background: transparent;
		padding: 0;
		color: inherit;
		text-align: left;
		cursor: pointer;
	}
	.source-copy {
		display: grid;
		min-width: 0;
	}
	.source-list strong,
	.source-list small {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.source-list strong {
		font-size: 0.73rem;
	}
	.source-list small {
		color: var(--muted);
		font-size: 0.6rem;
	}
	.source-select > span:last-child {
		border-radius: 999px;
		background: color-mix(in oklch, var(--success) 18%, transparent);
		padding: 0.2rem 0.4rem;
		color: var(--success);
		font-size: 0.58rem;
		font-weight: 800;
	}
	.source-select > span.off {
		background: var(--surface-raised);
		color: var(--muted);
	}
	.source-actions {
		display: none;
		grid-template-columns: 1fr 1fr;
		gap: 0.35rem;
	}
	.source-list article:hover .source-actions,
	.source-list article.selected .source-actions {
		display: grid;
	}
	.source-actions button {
		display: flex;
		min-height: 2.3rem;
		align-items: center;
		justify-content: center;
		gap: 0.3rem;
		border: 1px solid var(--line);
		border-radius: 0.6rem;
		background: var(--surface-raised);
		font-size: 0.65rem;
		font-weight: 750;
		cursor: pointer;
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
	}
</style>
