<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { createQuery } from '@tanstack/svelte-query';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		Command,
		Search,
		X,
		House,
		ListVideo,
		Radio,
		PanelsTopLeft,
		Database,
		BookOpen,
		Settings,
		RefreshCw,
		UploadCloud,
		Play
	} from '@lucide/svelte';
	import { api, params } from '$lib/api/client';
	import type { GuideChannel, Paginated } from '$lib/api/types';

	let {
		compact = false,
		iconOnly = false,
		triggerLabel = 'Open command menu'
	} = $props<{
		compact?: boolean;
		iconOnly?: boolean;
		triggerLabel?: string;
	}>();
	let open = $state(false);
	let query = $state('');
	let status = $state('');
	let studio = $derived(page.url.pathname.startsWith('/studio'));
	let currentLineupId = $derived.by(() => {
		const match = page.url.pathname.match(/^\/studio\/lineups\/(\d+)/);
		return match ? Number(match[1]) : null;
	});
	const watchEntries = [
		{ label: 'Watch home', detail: 'Browse what is on now', href: '/', icon: House },
		{ label: 'Live guide', detail: 'Open the time-based guide', href: '/guide', icon: ListVideo },
		{ label: 'Channel directory', detail: 'Find a channel', href: '/channels', icon: Radio }
	];
	const studioEntries = [
		{
			label: 'Studio overview',
			detail: 'Setup health and publishing',
			href: '/studio',
			icon: PanelsTopLeft
		},
		{
			label: 'Sources',
			detail: 'Playlists and refresh jobs',
			href: '/studio/sources',
			icon: Database
		},
		{
			label: 'Guide data',
			detail: 'EPG coverage and logos',
			href: '/studio/guide-data',
			icon: BookOpen
		},
		{
			label: 'Settings',
			detail: 'Matching, streaming and server',
			href: '/studio/settings',
			icon: Settings
		}
	];
	let entries = $derived(
		studio ? [...studioEntries, ...watchEntries] : [...watchEntries, ...studioEntries]
	);
	let filtered = $derived(
		entries.filter((entry) =>
			`${entry.label} ${entry.detail}`.toLowerCase().includes(query.toLowerCase())
		)
	);
	const channelSearch = createQuery(() => ({
		queryKey: ['command-search', query],
		enabled: open && query.trim().length >= 2,
		queryFn: () =>
			api<Paginated<GuideChannel>>(`/api/v2/search${params({ q: query.trim(), limit: 8 })}`),
		staleTime: 15_000
	}));
	function choose(href: string) {
		open = false;
		query = '';
		void goto(href);
	}
	async function publishCurrent() {
		if (!currentLineupId) return;
		status = 'Queuing publication…';
		try {
			await api(`/api/v2/studio/lineups/${currentLineupId}/publish`, { method: 'POST' });
			status = 'Publication queued.';
			setTimeout(() => (open = false), 500);
		} catch {
			status = 'Publication could not be queued.';
		}
	}
	function refreshView() {
		open = false;
		location.reload();
	}
	onMount(() => {
		const handler = (event: KeyboardEvent) => {
			if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
				event.preventDefault();
				open = !open;
			}
		};
		window.addEventListener('keydown', handler);
		return () => window.removeEventListener('keydown', handler);
	});
</script>

<button
	class:compact
	class:icon-only={iconOnly}
	class="command-trigger"
	onclick={() => (open = true)}
	aria-label={triggerLabel}
	title={iconOnly ? triggerLabel : undefined}
>
	<Search size={18} />
	<span>Find anything</span>
	<kbd><Command size={12} />K</kbd>
</button>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="command-overlay" />
		<Dialog.Content class="command-dialog" aria-describedby="command-description">
			<div class="command-header">
				<Search size={20} /><input
					bind:value={query}
					placeholder="Channels, pages, actions…"
					aria-label="Search commands"
				/><Dialog.Close class="command-close" aria-label="Close"><X size={20} /></Dialog.Close>
			</div>
			<Dialog.Title class="sr-only">Xivi command menu</Dialog.Title><Dialog.Description
				id="command-description"
				class="sr-only">Search for Xivi pages and actions.</Dialog.Description
			>
			<div class="command-results">
				<div class="command-actions">
					{#if currentLineupId}
						<button onclick={publishCurrent}>
							<UploadCloud size={19} />
							<span>
								<strong>Publish this lineup</strong>
								<small>Build M3U and XMLTV outputs</small>
							</span>
						</button>
					{/if}
					<button onclick={refreshView}>
						<RefreshCw size={19} />
						<span>
							<strong>Refresh this view</strong>
							<small>Reload current data and status</small>
						</span>
					</button>
				</div>
				{#if status}<p class="command-status" role="status">{status}</p>{/if}
				{#if filtered.length}
					{#each filtered as entry}
						{@const Icon = entry.icon}
						<button onclick={() => choose(entry.href)}>
							<Icon size={19} />
							<span>
								<strong>{entry.label}</strong>
								<small>{entry.detail}</small>
							</span>
						</button>
					{/each}
				{/if}
				{#if channelSearch.data?.items.length}
					<p class="result-label">Channels</p>
					{#each channelSearch.data.items as channel}
						<button onclick={() => choose(`/watch/channel/${channel.id}`)}>
							<Play size={18} />
							<span>
								<strong>{channel.name}</strong>
								<small>
									{channel.group_name} · {channel.current?.title ?? 'Schedule unavailable'}
								</small>
							</span>
						</button>
					{/each}
				{:else if query.length >= 2 && !filtered.length && !channelSearch.isPending}
					<p class="empty">No results. Try “guide” or “sources”.</p>
				{/if}
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.command-trigger {
		display: flex;
		min-height: 2.75rem;
		min-width: min(23rem, 34vw);
		align-items: center;
		gap: 0.65rem;
		border: 1px solid var(--line);
		border-radius: 0.85rem;
		background: var(--surface);
		padding: 0.5rem 0.65rem 0.5rem 0.85rem;
		color: var(--muted);
		cursor: pointer;
	}
	.command-trigger span {
		flex: 1;
		text-align: left;
	}
	.command-trigger.compact {
		width: 100%;
		min-width: 0;
		justify-content: flex-start;
		background: transparent;
	}
	.command-trigger.compact kbd {
		margin-left: auto;
	}
	.command-trigger.compact.icon-only {
		width: 2.75rem;
		min-width: 2.75rem;
		justify-content: center;
		padding: 0;
	}
	.command-trigger.compact.icon-only span,
	.command-trigger.compact.icon-only kbd {
		display: none;
	}
	kbd {
		display: inline-flex;
		align-items: center;
		gap: 0.15rem;
		border: 1px solid var(--line);
		border-radius: 0.4rem;
		padding: 0.2rem 0.38rem;
		font-size: 0.67rem;
	}
	:global(.command-overlay) {
		position: fixed;
		z-index: 80;
		inset: 0;
		background: rgb(8 10 15 / 0.72);
		animation: fade var(--micro) ease-out;
	}
	:global(.command-dialog) {
		position: fixed;
		z-index: 81;
		top: 14vh;
		left: 50%;
		width: min(42rem, calc(100vw - 2rem));
		transform: translateX(-50%);
		overflow: hidden;
		border: 1px solid var(--line);
		border-radius: 1.2rem;
		background: var(--surface-raised);
		box-shadow: 0 28px 90px rgb(0 0 0 / 0.45);
	}
	.command-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		border-bottom: 1px solid var(--line);
		padding: 0.8rem 1rem;
	}
	.command-header input {
		flex: 1;
		min-height: 2.5rem;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
		font-size: 1.05rem;
	}
	:global(.command-close) {
		display: grid;
		width: 2.5rem;
		height: 2.5rem;
		place-items: center;
		border: 0;
		border-radius: 0.7rem;
		background: transparent;
		cursor: pointer;
	}
	.command-results {
		max-height: 56vh;
		overflow: auto;
		padding: 0.55rem;
	}
	.command-results button {
		display: flex;
		width: 100%;
		min-height: 3.7rem;
		align-items: center;
		gap: 0.8rem;
		border: 0;
		border-radius: 0.8rem;
		background: transparent;
		padding: 0.7rem 0.85rem;
		text-align: left;
		cursor: pointer;
	}
	.command-results button:hover,
	.command-results button:focus-visible {
		background: color-mix(in oklch, var(--aqua) 13%, transparent);
	}
	.command-actions {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.35rem;
		border-bottom: 1px solid var(--line);
		padding-bottom: 0.55rem;
	}
	.command-actions button {
		min-height: 3.2rem;
		background: var(--surface);
	}
	.command-status {
		margin: 0.45rem;
		border-radius: 0.55rem;
		background: var(--sun);
		padding: 0.55rem 0.7rem;
		color: var(--ink);
		font-size: 0.7rem;
		font-weight: 750;
	}
	.result-label {
		margin: 0.75rem 0.85rem 0.25rem;
		color: var(--muted);
		font-size: 0.62rem;
		font-weight: 850;
		letter-spacing: 0.1em;
		text-transform: uppercase;
	}
	.command-results span {
		display: grid;
	}
	.command-results strong {
		font-size: 0.9rem;
	}
	.command-results small {
		color: var(--muted);
	}
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--muted);
	}
	@keyframes fade {
		from {
			opacity: 0;
		}
	}
	@media (max-width: 760px) {
		.command-trigger {
			min-width: 2.75rem;
			width: 2.75rem;
			justify-content: center;
			padding: 0;
		}
		.command-trigger span,
		.command-trigger kbd {
			display: none;
		}
		:global(.command-dialog) {
			top: 1rem;
		}
	}
	@media (max-width: 900px) {
		.command-trigger.compact {
			width: 2.75rem;
			justify-content: center;
			padding: 0;
		}
		.command-trigger.compact span,
		.command-trigger.compact kbd {
			display: none;
		}
	}
</style>
