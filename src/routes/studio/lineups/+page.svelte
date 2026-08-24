<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { Plus, Rows3, Trash2, ArrowRight, Radio } from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type { LineupSummary, Paginated } from '$lib/api/types';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	const client = useQueryClient();
	const query = createQuery(() => ({
		queryKey: ['studio', 'lineups'],
		queryFn: () => api<Paginated<LineupSummary>>('/api/v2/studio/lineups')
	}));
	let creating = $state(false),
		name = $state(''),
		message = $state('');
	async function createLineup() {
		if (!name.trim()) return;
		creating = true;
		message = '';
		try {
			await api('/api/template', { method: 'POST', body: JSON.stringify({ name: name.trim() }) });
			name = '';
			await client.invalidateQueries({ queryKey: ['studio', 'lineups'] });
		} catch {
			message = 'That lineup could not be created. Its name may already be in use.';
		} finally {
			creating = false;
		}
	}
	async function remove(lineup: LineupSummary) {
		if (
			!confirm(
				`Delete “${lineup.name}”? Its channels and source data will remain available where shared.`
			)
		)
			return;
		try {
			await api(`/api/template/${lineup.id}`, { method: 'DELETE' });
			await client.invalidateQueries({ queryKey: ['studio', 'lineups'] });
		} catch {
			message = 'The lineup could not be deleted.';
		}
	}
</script>

<svelte:head><title>Lineups · Xivi Studio</title></svelte:head>
<StudioHeader
	title="Lineups"
	description="Reusable, ordered channel collections that power Watch, M3U, XMLTV, and compatible devices."
>
	<form
		class="create-lineup"
		onsubmit={(event) => {
			event.preventDefault();
			void createLineup();
		}}
	>
		<label
			><span class="sr-only">New lineup name</span><input
				bind:value={name}
				placeholder="New lineup name"
			/></label
		><button class="app-button app-button--primary" disabled={creating || !name.trim()}
			><Plus size={18} />Create</button
		>
	</form>
</StudioHeader>
{#if message}<p class="lineup-message" role="status">{message}</p>{/if}
{#if query.isPending}<div class="lineup-grid">
		{#each Array(4) as _}<div class="skeleton"></div>{/each}
	</div>
{:else if query.isError}<EmptyState
		title="Lineups are unavailable"
		message="Xivi could not load the workspace. Try again from the overview."
		action="Studio overview"
		href="/studio"
	/>
{:else if !query.data?.total}<EmptyState
		title="Start with one lineup"
		message="A lineup turns source channels into an ordered guide and publishable output."
		action="Focus name field"
		href="/studio/lineups"
	/>
{:else}<div class="lineup-grid">
		{#each query.data.items as lineup}<article>
				<div class="lineup-icon"><Rows3 size={26} /></div>
				<div class="lineup-main">
					<span class="eyebrow">Lineup</span>
					<h2>{lineup.name}</h2>
					<dl>
						<div>
							<dt>Groups</dt>
							<dd>{lineup.group_count}</dd>
						</div>
						<div>
							<dt>Channels</dt>
							<dd>{lineup.channel_count}</dd>
						</div>
						<div>
							<dt>Guide</dt>
							<dd>{lineup.epg_coverage}%</dd>
						</div>
					</dl>
				</div>
				<div class="lineup-actions">
					<a class="app-button app-button--secondary" href={`/studio/lineups/${lineup.id}`}
						>Open workbench <ArrowRight size={17} /></a
					><a
						class="app-button app-button--quiet app-button--icon"
						href={`/?lineup=${lineup.id}`}
						aria-label={`Watch ${lineup.name}`}><Radio size={18} /></a
					><button
						class="app-button app-button--quiet app-button--icon"
						onclick={() => remove(lineup)}
						aria-label={`Delete ${lineup.name}`}><Trash2 size={18} /></button
					>
				</div>
			</article>{/each}
	</div>{/if}

<style>
	.create-lineup {
		display: flex;
		gap: 0.5rem;
	}
	.create-lineup input {
		min-height: 2.75rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface);
		padding: 0.6rem 0.8rem;
		color: var(--text);
	}
	.lineup-message {
		margin: 0 2.5rem 1rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface);
		padding: 0.7rem 1rem;
		color: var(--muted);
	}
	.lineup-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(25rem, 1fr));
		gap: 1rem;
		padding: 0 clamp(1.4rem, 3vw, 2.5rem) 3rem;
	}
	.lineup-grid > div.skeleton {
		height: 16rem;
		border-radius: 1.2rem;
	}
	.lineup-grid article {
		display: grid;
		min-height: 16rem;
		grid-template-columns: auto 1fr;
		gap: 1rem;
		border: 1px solid var(--line);
		border-radius: 1.2rem;
		background: var(--surface);
		padding: 1.1rem;
	}
	.lineup-icon {
		display: grid;
		width: 3.4rem;
		height: 3.4rem;
		place-items: center;
		border-radius: 0.9rem;
		background: var(--periwinkle);
		color: #fff;
	}
	.lineup-main h2 {
		margin: 0.1rem 0 1.4rem;
		font-size: 1.7rem;
	}
	.lineup-main dl {
		display: flex;
		gap: 1.6rem;
	}
	.lineup-main dl > div {
		display: grid;
	}
	.lineup-main dt {
		color: var(--muted);
		font-size: 0.67rem;
	}
	.lineup-main dd {
		margin: 0;
		font-family: var(--font-display);
		font-size: 1.4rem;
	}
	.lineup-actions {
		display: flex;
		grid-column: 1/3;
		align-items: center;
		gap: 0.35rem;
		margin-top: auto;
	}
	.lineup-actions > a:first-child {
		flex: 1;
	}
	@media (max-width: 620px) {
		.create-lineup {
			display: grid;
		}
		.lineup-grid {
			grid-template-columns: 1fr;
			padding-inline: 1rem;
		}
		.lineup-grid article {
			min-height: 14rem;
		}
		.lineup-main dl {
			gap: 1rem;
		}
	}
</style>
