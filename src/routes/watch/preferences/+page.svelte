<script lang="ts">
	import { onMount } from 'svelte';
	import { api, XiviAPIError } from '$lib/api/client';
	import type { GuideChannel, LineupSummary } from '$lib/api/types';
	type FavoriteList = { id: string; name: string; channels: string[] };
	type Preferences = {
		revision: number;
		favorites: FavoriteList[];
		hidden: string[];
		order: string[];
	};
	type Channel = GuideChannel & { key: string; lineup: number };
	let preferences = $state<Preferences>({ revision: 0, favorites: [], hidden: [], order: [] });
	let lineups = $state<LineupSummary[]>([]);
	let channels = $state<Channel[]>([]);
	let lineup = $state(0);
	let list = $state('');
	let newName = $state('');
	let query = $state('');
	let visibleCount = $state(100);
	let status = $state('');
	let busy = $state(false);
	let loading = $state(false);
	let generation = 0;
	let currentList = $derived(preferences.favorites.find((item) => item.id === list));
	let ordered = $derived.by(() => {
		const ranks = new Map(preferences.order.map((key, index) => [key, index]));
		return [...channels].sort(
			(a, b) =>
				(ranks.get(a.key) ?? Number.MAX_SAFE_INTEGER) -
					(ranks.get(b.key) ?? Number.MAX_SAFE_INTEGER) || a.number - b.number
		);
	});
	let filtered = $derived(
		ordered.filter((channel) =>
			`${channel.number} ${channel.name}`.toLowerCase().includes(query.toLowerCase())
		)
	);
	onMount(() => {
		void initialize();
	});
	async function initialize() {
		try {
			const [saved, available] = await Promise.all([
				api<Preferences>('/api/v2/watch/preferences'),
				api<{ items: LineupSummary[] }>('/api/v2/watch/lineups')
			]);
			preferences = saved;
			lineups = available.items;
			list = saved.favorites[0]?.id ?? '';
			lineup = lineups[0]?.id ?? 0;
			if (lineup) await loadChannels();
		} catch (e) {
			status = e instanceof Error ? e.message : 'Could not load viewer preferences.';
		}
	}
	async function loadChannels() {
		const run = ++generation;
		const selected = lineup;
		channels = [];
		loading = true;
		visibleCount = 100;
		let cursor: string | null = null;
		let restarted = false;
		try {
			do {
				let page: { items: GuideChannel[]; next_cursor: string | null };
				try {
					page = await api(
						`/api/v2/watch/lineups/${selected}/channels?metadata_only=true&limit=250${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`
					);
				} catch (e) {
					if (e instanceof XiviAPIError && e.detail.code === 'invalid_cursor' && !restarted) {
						cursor = null;
						restarted = true;
						continue;
					}
					throw e;
				}
				if (run !== generation) return;
				const all = new Map(channels.map((channel) => [channel.key, channel]));
				for (const channel of page.items)
					all.set(`${selected}:${channel.id}`, {
						...channel,
						key: `${selected}:${channel.id}`,
						lineup: selected
					});
				channels = [...all.values()];
				cursor = page.next_cursor;
			} while (cursor);
		} catch (e) {
			status = e instanceof Error ? e.message : 'Could not load channels.';
		} finally {
			if (run === generation) loading = false;
		}
	}
	async function save(next: Preferences) {
		if (busy) return;
		busy = true;
		status = '';
		try {
			preferences = await api('/api/v2/watch/preferences', {
				method: 'PATCH',
				body: JSON.stringify(next)
			});
			status = 'Saved to your account.';
		} catch (e) {
			if (e instanceof XiviAPIError && e.status === 409) {
				preferences = await api('/api/v2/watch/preferences');
				status =
					'Another device changed these preferences. Its changes are now loaded; please try your edit again.';
			} else status = e instanceof Error ? e.message : 'Could not save preferences.';
		} finally {
			busy = false;
		}
	}
	function favorite(key: string) {
		if (!currentList) return;
		const target = currentList;
		void save({
			...preferences,
			favorites: preferences.favorites.map((item) =>
				item.id === target.id
					? {
							...item,
							channels: item.channels.includes(key)
								? item.channels.filter((id) => id !== key)
								: [...item.channels, key]
						}
					: item
			)
		});
	}
	function hide(key: string) {
		void save({
			...preferences,
			hidden: preferences.hidden.includes(key)
				? preferences.hidden.filter((id) => id !== key)
				: [...preferences.hidden, key]
		});
	}
	function move(key: string, delta: number) {
		const order = [...new Set([...preferences.order, ...ordered.map((item) => item.key)])];
		const from = order.indexOf(key);
		const to = Math.max(0, Math.min(order.length - 1, from + delta));
		order.splice(from, 1);
		order.splice(to, 0, key);
		void save({ ...preferences, order });
	}
	async function createList(event: SubmitEvent) {
		event.preventDefault();
		if (!newName.trim()) return;
		const id = crypto.randomUUID();
		await save({
			...preferences,
			favorites: [...preferences.favorites, { id, name: newName.trim(), channels: [] }]
		});
		if (preferences.favorites.some((item) => item.id === id)) {
			list = id;
			newName = '';
		}
	}
</script>

<svelte:head><title>Watch preferences · Xivi</title></svelte:head>
<section class="viewer-preferences">
	<h1>Your channel lists</h1>
	<p>Favorites, hidden channels and ordering sync to your TVs. Studio lineups are unchanged.</p>
	{#if status}<p role="status">{status}</p>{/if}
	<div class="tools">
		<label
			>Lineup <select bind:value={lineup} onchange={() => loadChannels()}
				>{#each lineups as item}<option value={item.id}>{item.name}</option>{/each}</select
			></label
		>
		<label
			>Favorites list <select bind:value={list}
				><option value="">Choose a list</option>{#each preferences.favorites as item}<option
						value={item.id}>{item.name}</option
					>{/each}</select
			></label
		>
		<label
			>Filter channels <input
				bind:value={query}
				oninput={() => (visibleCount = 100)}
				placeholder="Name or number"
			/></label
		>
	</div>
	<form onsubmit={createList}>
		<label>New list name <input bind:value={newName} maxlength="80" required /></label><button
			class="app-button"
			disabled={busy || preferences.favorites.length >= 20}>Create list</button
		>
	</form>
	{#if currentList}<div class="tools">
			<button
				class="app-button app-button--quiet"
				disabled={busy}
				onclick={() => {
					void save({
						...preferences,
						favorites: preferences.favorites.filter((item) => item.id !== list)
					});
					list = '';
				}}>Delete “{currentList.name}” list</button
			>
		</div>{/if}
	<p>
		{channels.length} channels loaded{loading ? ' • loading more…' : ''}. Changes save
		automatically.
	</p>
	<ul>
		{#each filtered.slice(0, visibleCount) as channel (channel.key)}<li
				class:hidden-channel={preferences.hidden.includes(channel.key)}
			>
				<div class="identity">
					<strong>{channel.number} · {channel.name}</strong><small>{channel.group_name}</small>
				</div>
				<div class="actions">
					<button
						disabled={busy || !currentList}
						aria-pressed={currentList?.channels.includes(channel.key) ?? false}
						aria-label={`Favorite ${channel.name}`}
						onclick={() => favorite(channel.key)}
						>{currentList?.channels.includes(channel.key) ? '★ Favorited' : '☆ Favorite'}</button
					><button disabled={busy} onclick={() => hide(channel.key)}
						>{preferences.hidden.includes(channel.key) ? 'Unhide' : 'Hide'}</button
					><button
						disabled={busy || loading}
						aria-label={`Move ${channel.name} up`}
						onclick={() => move(channel.key, -1)}>↑</button
					><button
						disabled={busy || loading}
						aria-label={`Move ${channel.name} down`}
						onclick={() => move(channel.key, 1)}>↓</button
					>
				</div>
			</li>{/each}
	</ul>
	{#if visibleCount < filtered.length}<button
			class="app-button"
			onclick={() => (visibleCount += 100)}>Show 100 more channels</button
		>{/if}
	<div class="tools">
		<button
			class="app-button app-button--quiet"
			disabled={busy}
			onclick={() => save({ ...preferences, order: [] })}>Restore lineup ordering</button
		><button
			class="app-button app-button--quiet"
			disabled={busy}
			onclick={() => save({ ...preferences, hidden: [] })}>Unhide all channels</button
		>
	</div>
	<p>Recent history and display controls stay on each TV. Clearing caches never signs a TV out.</p>
</section>

<style>
	.viewer-preferences {
		max-width: 70rem;
		margin: 1rem auto;
		padding: 1.5rem;
	}
	.tools,
	form,
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: end;
		margin-block: 1rem;
	}
	label,
	.identity {
		display: grid;
		gap: 0.35rem;
	}
	input,
	select,
	.actions button {
		padding: 0.65rem;
		border: 1px solid var(--border);
		border-radius: 0.5rem;
		color: inherit;
		background: var(--panel);
	}
	ul {
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: center;
		justify-content: space-between;
		padding: 0.6rem;
		border-bottom: 1px solid var(--border);
	}
	.identity {
		flex: 1;
		min-width: 12rem;
	}
	.hidden-channel .identity {
		opacity: 0.55;
	}
	button:focus-visible {
		outline: 3px solid var(--accent);
		outline-offset: 3px;
	}
</style>
