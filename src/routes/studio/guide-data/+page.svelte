<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import {
		Plus,
		RefreshCw,
		Trash2,
		Pencil,
		Search,
		Upload,
		BookOpenCheck,
		Image,
		CircleAlert,
		CheckCircle2
	} from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import type { CoverageSummary, LegacyEpg } from '$lib/api/types';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';

	type EpgResponse = { epgs: LegacyEpg[]; count: number };
	type Logo = { id: number; name: string; image: string };
	type LogoResponse = { logos: Logo[]; count: number };
	const client = useQueryClient();
	const epgQuery = createQuery(() => ({
		queryKey: ['guide-data', 'epgs'],
		queryFn: () => api<EpgResponse>('/api/epgs')
	}));
	const coverageQuery = createQuery(() => ({
		queryKey: ['guide-data', 'coverage'],
		queryFn: () => api<CoverageSummary>('/api/v2/studio/guide-data/coverage'),
		refetchInterval: 60_000
	}));
	const logoQuery = createQuery(() => ({
		queryKey: ['logos'],
		queryFn: () => api<LogoResponse>('/api/logos')
	}));
	let name = $state(''),
		url = $state(''),
		editing = $state<LegacyEpg | null>(null),
		busy = $state<number | 'form' | 'upload' | null>(null),
		message = $state(''),
		logoSearch = $state(''),
		diagnostics = $state<'missing' | 'unused'>('missing');
	let filteredLogos = $derived(
		(logoQuery.data?.logos ?? []).filter(
			(logo) => logo.id !== 0 && logo.name.toLowerCase().includes(logoSearch.toLowerCase())
		)
	);
	let diagnosticItems = $derived(
		diagnostics === 'missing'
			? (coverageQuery.data?.channels_without_tvg_id ?? [])
			: (coverageQuery.data?.unmapped_epg_ids ?? [])
	);
	function reset() {
		name = '';
		url = '';
		editing = null;
	}
	function edit(epg: LegacyEpg) {
		editing = epg;
		name = epg.name;
		url = '';
	}
	async function save() {
		if (!name.trim() || (!editing && !url.trim())) return;
		busy = 'form';
		try {
			await api('/api/epg', {
				method: editing ? 'PUT' : 'POST',
				body: JSON.stringify({
					...(editing ? { id: editing.id, orderr: editing.orderr } : {}),
					name: name.trim(),
					url: url.trim(),
					created_at: editing?.created_at,
					updated_at: new Date().toISOString()
				})
			});
			message = editing ? 'Guide source updated.' : 'Guide source added and import started.';
			reset();
			await client.invalidateQueries({ queryKey: ['guide-data'] });
		} catch {
			message = 'The guide source could not be saved.';
		} finally {
			busy = null;
		}
	}
	async function refresh(epg: LegacyEpg) {
		busy = epg.id;
		try {
			await api(`/api/v2/studio/guide-data/sources/${epg.id}/refresh`, { method: 'POST' });
			message = `Refresh queued for ${epg.name}.`;
			setTimeout(() => client.invalidateQueries({ queryKey: ['guide-data'] }), 3000);
		} catch {
			message = 'The guide refresh could not be started.';
		} finally {
			busy = null;
		}
	}
	async function removeEpg(epg: LegacyEpg) {
		if (!confirm(`Delete “${epg.name}” and its imported schedule data?`)) return;
		try {
			await api(`/api/epg/${epg.id}`, { method: 'DELETE' });
			await client.invalidateQueries({ queryKey: ['guide-data'] });
			message = 'Guide source deleted.';
		} catch {
			message = 'The guide source could not be deleted.';
		}
	}
	async function upload(event: Event) {
		const file = (event.currentTarget as HTMLInputElement).files?.[0];
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			message = 'Choose an image file.';
			return;
		}
		busy = 'upload';
		const reader = new FileReader();
		reader.onload = async () => {
			try {
				const logoName = file.name.replace(/\.[^.]+$/, '').replace(/[^a-zA-Z0-9_-]+/g, '-');
				await api('/api/logo', {
					method: 'POST',
					body: JSON.stringify({ name: logoName, image: String(reader.result) })
				});
				await client.invalidateQueries({ queryKey: ['logos'] });
				message = `${logoName} uploaded.`;
			} catch {
				message = 'The logo could not be uploaded.';
			} finally {
				busy = null;
			}
		};
		reader.readAsDataURL(file);
	}
	async function removeLogo(logo: Logo) {
		if (
			!confirm(`Delete logo “${logo.name}”? Channels using it will fall back to the Signal Tile.`)
		)
			return;
		try {
			await api(`/api/logo/${logo.id}`, { method: 'DELETE' });
			await client.invalidateQueries({ queryKey: ['logos'] });
			message = 'Logo deleted.';
		} catch {
			message = 'The logo could not be deleted.';
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

<svelte:head><title>Guide Data · Xivi Studio</title></svelte:head>
<StudioHeader
	title="Guide data"
	description="EPG imports, coverage diagnostics, channel identifiers, and the reusable logo library."
/>
{#if message}<div class="guide-message" role="status">
		{message}<button onclick={() => (message = '')}>Dismiss</button>
	</div>{/if}
<div class="guide-data-layout">
	<section class="coverage-strip">
		<article class="aqua">
			<BookOpenCheck size={22} /><span>Coverage</span><strong
				>{coverageQuery.data?.coverage ?? '—'}%</strong
			><small
				>{coverageQuery.data?.mapped_channel_count ?? 0} of {coverageQuery.data
					?.lineup_channel_count ?? 0} channels mapped</small
			>
		</article>
		<article class="sun">
			<CheckCircle2 size={22} /><span>EPG channels</span><strong
				>{coverageQuery.data?.epg_channel_count.toLocaleString() ?? '—'}</strong
			><small
				>{coverageQuery.data?.programme_count.toLocaleString() ?? 0} programmes available</small
			>
		</article>
		<article class="paper">
			<CircleAlert size={22} /><span>Without TVG ID</span><strong
				>{coverageQuery.data?.channels_without_tvg_id.length ?? '—'}</strong
			><small>Shown below for resolution</small>
		</article>
	</section>

	<section class="panel epg-editor">
		<header>
			<div>
				<p class="eyebrow">EPG sources</p>
				<h2>{editing ? `Edit ${editing.name}` : 'Add XMLTV source'}</h2>
			</div>
		</header>
		<form
			onsubmit={(event) => {
				event.preventDefault();
				void save();
			}}
		>
			<label>Name<input bind:value={name} required placeholder="Regional guide" /></label><label
				>XMLTV URL<input
					bind:value={url}
					required={!editing}
					type="url"
					placeholder="https://provider.example/guide.xml"
				/></label
			>
			{#if editing}<small class="secret-note"
					>Leave this blank to keep the encrypted XMLTV URL. Enter a URL only to replace it.</small
				>{/if}
			<div>
				<button class="app-button app-button--primary" disabled={busy === 'form'}
					><Plus size={17} />{editing ? 'Save source' : 'Add and import'}</button
				>{#if editing}<button type="button" class="app-button app-button--quiet" onclick={reset}
						>Cancel</button
					>{/if}
			</div>
		</form>
	</section>
	<section class="panel epg-list">
		<header>
			<div>
				<p class="eyebrow">Connected guides</p>
				<h2>{epgQuery.data?.count ?? 0} XMLTV sources</h2>
			</div>
		</header>
		{#if epgQuery.isPending}<div
				class="list-loading skeleton"
			></div>{:else if !epgQuery.data?.epgs.length}<div class="empty-small">
				<p>No XMLTV sources are connected.</p>
			</div>{:else}<div>
				{#each epgQuery.data.epgs as epg}<article>
						<div>
							<strong>{epg.name}</strong><small>{epg.url}</small><time
								>Updated {format(epg.updated_at)}</time
							>
						</div>
						<button
							onclick={() => refresh(epg)}
							disabled={busy === epg.id}
							aria-label={`Refresh ${epg.name}`}
							><RefreshCw class={busy === epg.id ? 'spin' : undefined} size={17} /></button
						><button onclick={() => edit(epg)} aria-label={`Edit ${epg.name}`}
							><Pencil size={17} /></button
						><button onclick={() => removeEpg(epg)} aria-label={`Delete ${epg.name}`}
							><Trash2 size={17} /></button
						>
					</article>{/each}
			</div>{/if}
	</section>

	<section class="panel diagnostics">
		<header>
			<div>
				<p class="eyebrow">Coverage diagnostics</p>
				<h2>Resolve identifiers</h2>
			</div>
			<div class="diag-toggle">
				<button class:active={diagnostics === 'missing'} onclick={() => (diagnostics = 'missing')}
					>Channels without TVG ID</button
				><button class:active={diagnostics === 'unused'} onclick={() => (diagnostics = 'unused')}
					>Unused EPG IDs</button
				>
			</div>
		</header>
		<div class="identifier-list">
			{#each diagnosticItems as identifier}<span>{identifier}</span>{:else}<div class="empty-small">
					<CheckCircle2 size={20} />
					<p>No issues in this category.</p>
				</div>{/each}
		</div>
	</section>

	<section class="panel logos">
		<header>
			<div>
				<p class="eyebrow">Asset library</p>
				<h2>{logoQuery.data?.count ? logoQuery.data.count - 1 : 0} channel logos</h2>
			</div>
			<label class="upload-button"
				><Upload size={16} />{busy === 'upload' ? 'Uploading…' : 'Upload logo'}<input
					type="file"
					accept="image/*"
					onchange={upload}
					disabled={busy === 'upload'}
				/></label
			>
		</header>
		<label class="logo-search"
			><Search size={18} /><input bind:value={logoSearch} placeholder="Search logos" /></label
		>
		<div class="logo-grid">
			{#each filteredLogos as logo}<article>
					<LogoTile
						src={logo.image.startsWith('/') ? logo.image : `/${logo.image}`}
						name={logo.name}
						size="md"
						contrast
					/>
					<strong>{logo.name}</strong><button
						onclick={() => removeLogo(logo)}
						aria-label={`Delete ${logo.name}`}><Trash2 size={15} /></button
					>
				</article>{:else}<div class="empty-small">
					<Image size={22} />
					<p>No logos match.</p>
				</div>{/each}
		</div>
	</section>
</div>

<style>
	.guide-message {
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
	.guide-message button {
		border: 0;
		background: transparent;
		text-decoration: underline;
		cursor: pointer;
	}
	.guide-data-layout {
		display: grid;
		grid-template-columns: minmax(18rem, 0.65fr) minmax(30rem, 1.35fr);
		gap: 1rem;
		padding: 0 clamp(1.4rem, 3vw, 2.5rem) 3rem;
	}
	.coverage-strip {
		display: grid;
		grid-column: 1/3;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}
	.coverage-strip article {
		display: grid;
		grid-template-columns: 1fr auto;
		border-radius: 1rem;
		padding: 1rem;
		color: var(--ink);
	}
	.coverage-strip .aqua {
		background: var(--aqua);
	}
	.coverage-strip .sun {
		background: var(--sun);
	}
	.coverage-strip .paper {
		background: var(--paper);
	}
	.coverage-strip article > span {
		font-weight: 800;
	}
	.coverage-strip article > strong {
		grid-column: 1/3;
		margin: 0.8rem 0 0.1rem;
		font-family: var(--font-display);
		font-size: 2.5rem;
	}
	.coverage-strip article > small {
		grid-column: 1/3;
	}
	.panel {
		overflow: hidden;
	}
	.panel > header {
		display: flex;
		min-height: 4.5rem;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--line);
		padding: 0.8rem 1rem;
	}
	.panel h2 {
		margin: 0;
		font-size: 1.25rem;
	}
	.epg-editor form {
		display: grid;
		gap: 0.75rem;
		padding: 1rem;
	}
	.epg-editor label {
		display: grid;
		gap: 0.3rem;
		color: var(--muted);
		font-size: 0.7rem;
	}
	.epg-editor input {
		min-height: 2.7rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface-raised);
		padding: 0.55rem 0.7rem;
		color: var(--text);
	}
	.epg-editor form > div {
		display: flex;
		gap: 0.4rem;
	}
	.epg-list article {
		display: grid;
		grid-template-columns: minmax(0, 1fr) repeat(3, 2.4rem);
		align-items: center;
		gap: 0.3rem;
		border-bottom: 1px solid var(--line);
		padding: 0.7rem 0.8rem;
	}
	.epg-list article > div {
		display: grid;
		min-width: 0;
	}
	.epg-list strong,
	.epg-list small,
	.epg-list time {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.epg-list strong {
		font-size: 0.75rem;
	}
	.epg-list small,
	.epg-list time {
		color: var(--muted);
		font-size: 0.6rem;
	}
	.epg-list button,
	.logos article button {
		display: grid;
		width: 2.3rem;
		height: 2.3rem;
		place-items: center;
		border: 0;
		border-radius: 0.55rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.epg-list button:hover,
	.logos article button:hover {
		background: var(--surface-raised);
		color: var(--text);
	}
	.diagnostics,
	.logos {
		grid-column: 1/3;
	}
	.diag-toggle {
		display: flex;
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		padding: 0.2rem;
	}
	.diag-toggle button {
		min-height: 2.2rem;
		border: 0;
		border-radius: 0.45rem;
		background: transparent;
		padding: 0.35rem 0.55rem;
		color: var(--muted);
		font-size: 0.62rem;
		cursor: pointer;
	}
	.diag-toggle button.active {
		background: var(--periwinkle);
		color: #fff;
	}
	.identifier-list {
		display: flex;
		max-height: 15rem;
		flex-wrap: wrap;
		gap: 0.4rem;
		overflow: auto;
		padding: 1rem;
	}
	.identifier-list > span {
		border: 1px solid var(--line);
		border-radius: 99px;
		background: var(--surface-raised);
		padding: 0.35rem 0.55rem;
		font-size: 0.65rem;
	}
	.upload-button {
		display: flex;
		min-height: 2.5rem;
		align-items: center;
		gap: 0.4rem;
		border-radius: 0.65rem;
		background: var(--coral);
		padding: 0.5rem 0.7rem;
		color: var(--ink);
		font-size: 0.7rem;
		font-weight: 800;
		cursor: pointer;
	}
	.upload-button input {
		display: none;
	}
	.logo-search {
		display: flex;
		min-height: 2.8rem;
		align-items: center;
		gap: 0.5rem;
		border-bottom: 1px solid var(--line);
		padding: 0 1rem;
		color: var(--muted);
	}
	.logo-search input {
		width: 100%;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
	}
	.logo-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
		max-height: 30rem;
		overflow: auto;
	}
	.logos article {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.55rem;
		border-right: 1px solid var(--line);
		border-bottom: 1px solid var(--line);
		padding: 0.6rem;
	}
	.logos article strong {
		overflow: hidden;
		font-size: 0.68rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.empty-small {
		display: grid;
		min-height: 8rem;
		place-items: center;
		align-content: center;
		color: var(--muted);
		text-align: center;
	}
	.list-loading {
		height: 15rem;
	}
	@media (max-width: 900px) {
		.guide-data-layout {
			grid-template-columns: 1fr;
		}
		.coverage-strip,
		.diagnostics,
		.logos {
			grid-column: 1;
		}
		.coverage-strip {
			grid-template-columns: 1fr 1fr;
		}
		.coverage-strip article:last-child {
			grid-column: 1/3;
		}
	}
	@media (max-width: 600px) {
		.guide-data-layout {
			padding-inline: 1rem;
		}
		.guide-message {
			margin-inline: 1rem;
		}
		.coverage-strip {
			grid-template-columns: 1fr;
		}
		.coverage-strip article:last-child {
			grid-column: 1;
		}
		.panel > header {
			align-items: flex-start;
			flex-direction: column;
			gap: 0.7rem;
		}
		.diag-toggle {
			width: 100%;
		}
		.diag-toggle button {
			flex: 1;
		}
	}
</style>
