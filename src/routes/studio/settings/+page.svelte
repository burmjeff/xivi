<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import {
		Save,
		RotateCcw,
		Info,
		GitCompareArrows,
		RadioTower,
		Server,
		CalendarClock,
		Cast,
		Palette,
		HardDrive
	} from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';
	import { preferences, setTheme, type ThemePreference } from '$lib/state/preferences.svelte';
	const client = useQueryClient();

	type Settings = {
		application: {
			appname: string;
			appversion: string;
			tz: string;
			servepath: string;
			loglevel: number;
			updatecron: string;
		};
		server: { host: string; port: number; readtimeout: number };
		playlist: { tvgid_match: boolean; name_match: boolean; name_score: number };
		streaming: {
			proxy: boolean;
			ingest_buffer_ms: number;
			startup_timeout_seconds: number;
			stall_timeout_seconds: number;
			idle_timeout_seconds: number;
			retry_limit: number;
			retry_backoff_ms: number;
			hls_segment_seconds: number;
			hls_playlist_length: number;
			client_buffer_mb: number;
			tls_verify: boolean;
			useragent: string;
		};
		// Preserve legacy vector values while the settings endpoint still replaces the full document.
		vector: { batch_size: number; parallel_batches: number; timeout: number; cache_size: number };
		maintenance: {
			cleanup_interval_hours: number;
			operation_job_retention_days: number;
			stream_diagnostics_days: number;
			maximum_operation_jobs: number;
			maximum_stream_sessions: number;
		};
		virtual_tuner: {
			tuner_count: number;
		};
	};
	type Response = { settings: Settings };
	const query = createQuery(() => ({
		queryKey: ['settings'],
		queryFn: () => api<Response>('/api/settings')
	}));
	let settings = $state<Settings | null>(null),
		initialized = $state(false),
		saving = $state(''),
		message = $state(''),
		errors = $state<Record<string, string>>({});
	$effect(() => {
		if (query.data?.settings && !initialized) {
			settings = structuredClone(query.data.settings);
			initialized = true;
		}
	});
	function validate(section: string) {
		const next: Record<string, string> = {};
		if (!settings) return false;
		if (section === 'server') {
			if (!settings.server.host.trim()) next.host = 'Host is required.';
			if (settings.server.port < 1 || settings.server.port > 65535)
				next.port = 'Use a port from 1 to 65535.';
			if (settings.server.readtimeout < 1) next.readtimeout = 'Timeout must be positive.';
		}
		if (section === 'matching') {
			const score = settings.playlist.name_score;
			if (!Number.isFinite(score) || score < 0.01 || score > 1)
				next.name_score = 'Use a score from 0.01 to 1.';
		}
		if (section === 'streaming') {
			if (settings.streaming.ingest_buffer_ms < 0 || settings.streaming.ingest_buffer_ms > 30000)
				next.ingest_buffer_ms = 'Use a jitter buffer from 0 to 30,000 ms.';
			if (settings.streaming.startup_timeout_seconds < 3)
				next.startup_timeout_seconds = 'Allow at least 3 seconds for startup.';
			if (settings.streaming.stall_timeout_seconds < 3)
				next.stall_timeout_seconds = 'Allow at least 3 seconds before failover.';
			if (settings.streaming.idle_timeout_seconds < 10)
				next.idle_timeout_seconds = 'Allow at least 10 seconds before cleanup.';
			if (settings.streaming.retry_limit < 1)
				next.retry_limit = 'At least one source attempt is required.';
			if (settings.streaming.retry_backoff_ms < 100)
				next.retry_backoff_ms = 'Use a retry delay of at least 100 ms.';
			if (settings.streaming.hls_segment_seconds < 1 || settings.streaming.hls_segment_seconds > 10)
				next.hls_segment_seconds = 'Use HLS segments from 1 to 10 seconds.';
			if (settings.streaming.hls_playlist_length < 3)
				next.hls_playlist_length = 'Keep at least 3 segments in the live playlist.';
			if (settings.streaming.client_buffer_mb < 1)
				next.client_buffer_mb = 'Reserve at least 1 MB per MPEG-TS viewer.';
		}
		if (
			section === 'devices' &&
			(settings.virtual_tuner.tuner_count < 1 || settings.virtual_tuner.tuner_count > 255)
		)
			next.tuner_count = 'Use between 1 and 255 tuners.';
		if (section === 'scheduling' && settings.application.updatecron.trim().split(/\s+/).length < 5)
			next.updatecron = 'Enter a valid five- or six-field cron expression.';
		if (section === 'maintenance') {
			if (
				settings.maintenance.cleanup_interval_hours < 1 ||
				settings.maintenance.cleanup_interval_hours > 168
			)
				next.cleanup_interval_hours = 'Use an interval from 1 hour to 7 days.';
			if (
				settings.maintenance.operation_job_retention_days < 1 ||
				settings.maintenance.operation_job_retention_days > 365
			)
				next.operation_job_retention_days = 'Keep jobs from 1 to 365 days.';
			if (
				settings.maintenance.stream_diagnostics_days < 1 ||
				settings.maintenance.stream_diagnostics_days > 365
			)
				next.stream_diagnostics_days = 'Keep diagnostics from 1 to 365 days.';
			if (
				settings.maintenance.maximum_operation_jobs < 100 ||
				settings.maintenance.maximum_operation_jobs > 100000
			)
				next.maximum_operation_jobs = 'Keep between 100 and 100,000 jobs.';
			if (
				settings.maintenance.maximum_stream_sessions < 100 ||
				settings.maintenance.maximum_stream_sessions > 100000
			)
				next.maximum_stream_sessions = 'Keep between 100 and 100,000 sessions.';
		}
		errors = next;
		return !Object.keys(next).length;
	}
	async function save(section: string) {
		if (!settings || !validate(section)) return;
		saving = section;
		message = '';
		try {
			await api('/api/settings', { method: 'PUT', body: JSON.stringify(settings) });
			if (section === 'devices') {
				await client.invalidateQueries({ queryKey: ['studio', 'device-outputs'] });
			}
			message = `${section[0].toUpperCase()}${section.slice(1)} settings saved.${section === 'server' || section === 'maintenance' ? ' Restart Xivi to apply scheduling changes.' : ''}`;
		} catch {
			message = 'Settings could not be saved.';
		} finally {
			saving = '';
		}
	}
	function reset() {
		if (query.data?.settings) settings = structuredClone(query.data.settings);
		errors = {};
		message = 'Unsaved changes reset.';
	}
	const themes: ThemePreference[] = ['system', 'light', 'dark'];
</script>

<svelte:head><title>Settings · Xivi Studio</title></svelte:head>
<StudioHeader
	title="Settings"
	description="Change one area at a time. Xivi marks options that need a server restart."
	><button class="app-button app-button--secondary" onclick={reset}
		><RotateCcw size={17} />Reset unsaved</button
	></StudioHeader
>
{#if message}<div class="settings-message" role="status">
		{message}<button onclick={() => (message = '')}>Dismiss</button>
	</div>{/if}
{#if query.isPending}<div class="settings-loading">
		{#each Array(6) as _}<div class="skeleton"></div>{/each}
	</div>
{:else if query.isError || !settings}<div class="settings-error">
		<h2>Settings are unavailable</h2>
		<button class="app-button app-button--primary" onclick={() => query.refetch()}>Try again</button
		>
	</div>
{:else}<div class="settings-grid">
		<section class="settings-card">
			<header>
				<Info />
				<div>
					<p class="eyebrow">About</p>
					<h2>Application</h2>
				</div>
			</header>
			<div class="fields">
				<label>Application name<input bind:value={settings.application.appname} /></label><label
					>Version<input value={settings.application.appversion} disabled /></label
				><label
					>Display timezone<input
						bind:value={settings.application.tz}
						placeholder="America/New_York"
					/></label
				><label
					>Serve path <span class="restart">Restart</span><input
						bind:value={settings.application.servepath}
					/></label
				><label
					>Log level<input
						type="number"
						bind:value={settings.application.loglevel}
						min="0"
						max="5"
					/></label
				>
			</div>
			<footer>
				<button
					class="app-button app-button--primary"
					onclick={() => save('application')}
					disabled={saving === 'application'}><Save size={16} />Save About</button
				>
			</footer>
		</section>
		<section class="settings-card">
			<header>
				<GitCompareArrows />
				<div>
					<p class="eyebrow">Matching</p>
					<h2>Channel identity</h2>
				</div>
			</header>
			<div class="fields">
				<label class="switch-row"
					><span
						><strong>Match TVG IDs</strong><small>Prefer exact provider identifiers.</small></span
					><input type="checkbox" bind:checked={settings.playlist.tvgid_match} /></label
				><label class="switch-row"
					><span
						><strong>Match names</strong><small>Use normalized channel names as a fallback.</small
						></span
					><input type="checkbox" bind:checked={settings.playlist.name_match} /></label
				><label
					>Fuzzy auto-match threshold<input
						type="number"
						step="0.01"
						min="0.01"
						max="1"
						bind:value={settings.playlist.name_score}
					/>{#if errors.name_score}<em>{errors.name_score}</em>{/if}<small
						>Only affects fuzzy name candidates. Unique exact names bypass this threshold; a fuzzy
						match must also lead the runner-up by at least 0.05.</small
					></label
				>
			</div>
			<footer>
				<button
					class="app-button app-button--primary"
					onclick={() => save('matching')}
					disabled={saving === 'matching'}><Save size={16} />Save Matching</button
				>
			</footer>
		</section>
		<section class="settings-card">
			<header>
				<RadioTower />
				<div>
					<p class="eyebrow">Streaming</p>
					<h2>Playback pipeline</h2>
				</div>
			</header>
			<div class="fields">
				<label class="switch-row"
					><span
						><strong>Share proxied streams</strong><small
							>Use one resilient source connection for HLS and every MPEG-TS viewer.</small
						></span
					><input type="checkbox" bind:checked={settings.streaming.proxy} /></label
				><label class="switch-row"
					><span
						><strong>Verify source TLS</strong><small
							>Reject invalid HTTPS certificates instead of silently trusting them.</small
						></span
					><input type="checkbox" bind:checked={settings.streaming.tls_verify} /></label
				>
				<div class="field-pair">
					<label
						>Ingest jitter buffer (ms)<input
							type="number"
							min="0"
							max="30000"
							step="100"
							bind:value={settings.streaming.ingest_buffer_ms}
						/>{#if errors.ingest_buffer_ms}<em>{errors.ingest_buffer_ms}</em>{/if}<small
							>Absorbs provider jitter without delaying startup more than necessary.</small
						></label
					><label
						>Startup timeout (seconds)<input
							type="number"
							min="3"
							bind:value={settings.streaming.startup_timeout_seconds}
						/>{#if errors.startup_timeout_seconds}<em>{errors.startup_timeout_seconds}</em
							>{/if}</label
					><label
						>Stall failover (seconds)<input
							type="number"
							min="3"
							bind:value={settings.streaming.stall_timeout_seconds}
						/>{#if errors.stall_timeout_seconds}<em>{errors.stall_timeout_seconds}</em>{/if}</label
					><label
						>Idle cleanup (seconds)<input
							type="number"
							min="10"
							bind:value={settings.streaming.idle_timeout_seconds}
						/>{#if errors.idle_timeout_seconds}<em>{errors.idle_timeout_seconds}</em>{/if}</label
					><label
						>Source attempts<input
							type="number"
							min="1"
							bind:value={settings.streaming.retry_limit}
						/>{#if errors.retry_limit}<em>{errors.retry_limit}</em>{/if}</label
					><label
						>Base retry delay (ms)<input
							type="number"
							min="100"
							step="100"
							bind:value={settings.streaming.retry_backoff_ms}
						/>{#if errors.retry_backoff_ms}<em>{errors.retry_backoff_ms}</em>{/if}</label
					><label
						>HLS segment (seconds)<input
							type="number"
							min="1"
							max="10"
							bind:value={settings.streaming.hls_segment_seconds}
						/>{#if errors.hls_segment_seconds}<em>{errors.hls_segment_seconds}</em>{/if}<small
							>Two seconds balances startup speed, resilience, and disk activity.</small
						></label
					><label
						>HLS playlist segments<input
							type="number"
							min="3"
							bind:value={settings.streaming.hls_playlist_length}
						/>{#if errors.hls_playlist_length}<em>{errors.hls_playlist_length}</em>{/if}<small
							>Ten segments is recommended for jittery IPTV sources.</small
						></label
					><label
						>Per-viewer buffer (MB)<input
							type="number"
							min="1"
							bind:value={settings.streaming.client_buffer_mb}
						/>{#if errors.client_buffer_mb}<em>{errors.client_buffer_mb}</em>{/if}</label
					>
				</div>
				<label>User agent<input bind:value={settings.streaming.useragent} /></label>
			</div>
			<footer>
				<button
					class="app-button app-button--primary"
					onclick={() => save('streaming')}
					disabled={saving === 'streaming'}><Save size={16} />Save Streaming</button
				>
			</footer>
		</section>
		<section class="settings-card">
			<header>
				<Server />
				<div>
					<p class="eyebrow">Server</p>
					<h2>Network <span class="restart">Restart</span></h2>
				</div>
			</header>
			<div class="fields two">
				<label
					>Host<input bind:value={settings.server.host} />{#if errors.host}<em>{errors.host}</em
						>{/if}</label
				><label
					>Port<input
						type="number"
						min="1"
						max="65535"
						bind:value={settings.server.port}
					/>{#if errors.port}<em>{errors.port}</em>{/if}</label
				><label
					>Read timeout<input
						type="number"
						min="1"
						bind:value={settings.server.readtimeout}
					/>{#if errors.readtimeout}<em>{errors.readtimeout}</em>{/if}</label
				>
			</div>
			<footer>
				<button
					class="app-button app-button--primary"
					onclick={() => save('server')}
					disabled={saving === 'server'}><Save size={16} />Save Server</button
				>
			</footer>
		</section>
		<section class="settings-card">
			<header>
				<CalendarClock />
				<div>
					<p class="eyebrow">Scheduling</p>
					<h2>Automatic refresh</h2>
				</div>
			</header>
			<div class="fields">
				<label
					>Update cron<input
						bind:value={settings.application.updatecron}
						placeholder="0 */6 * * *"
					/>{#if errors.updatecron}<em>{errors.updatecron}</em>{/if}<small
						>Controls recurring playlist and guide refreshes.</small
					></label
				>
			</div>
			<footer>
				<button
					class="app-button app-button--primary"
					onclick={() => save('scheduling')}
					disabled={saving === 'scheduling'}><Save size={16} />Save Scheduling</button
				>
			</footer>
		</section>
		<section class="settings-card">
			<header>
				<HardDrive />
				<div>
					<p class="eyebrow">Maintenance</p>
					<h2>Storage retention</h2>
				</div>
			</header>
			<div class="fields">
				<p class="section-note">
					Cleanup always protects active jobs and streams. Guide data older than 24 hours, orphaned
					generated vectors, crash-left stream segments, and stale temporary files are removed
					automatically. Each retained stream session is also capped at 1,000 events and 1,000
					viewer records.
				</p>
				<div class="field-pair">
					<label
						>Cleanup interval (hours) <span class="restart">Restart</span><input
							type="number"
							min="1"
							max="168"
							bind:value={settings.maintenance.cleanup_interval_hours}
						/>{#if errors.cleanup_interval_hours}<em>{errors.cleanup_interval_hours}</em
							>{/if}</label
					>
					<label
						>Job history (days)<input
							type="number"
							min="1"
							max="365"
							bind:value={settings.maintenance.operation_job_retention_days}
						/>{#if errors.operation_job_retention_days}<em>{errors.operation_job_retention_days}</em
							>{/if}</label
					>
					<label
						>Stream diagnostics (days)<input
							type="number"
							min="1"
							max="365"
							bind:value={settings.maintenance.stream_diagnostics_days}
						/>{#if errors.stream_diagnostics_days}<em>{errors.stream_diagnostics_days}</em
							>{/if}</label
					>
					<label
						>Maximum completed jobs<input
							type="number"
							min="100"
							max="100000"
							step="100"
							bind:value={settings.maintenance.maximum_operation_jobs}
						/>{#if errors.maximum_operation_jobs}<em>{errors.maximum_operation_jobs}</em
							>{/if}</label
					>
					<label
						>Maximum completed sessions<input
							type="number"
							min="100"
							max="100000"
							step="100"
							bind:value={settings.maintenance.maximum_stream_sessions}
						/>{#if errors.maximum_stream_sessions}<em>{errors.maximum_stream_sessions}</em
							>{/if}</label
					>
				</div>
			</div>
			<footer>
				<button
					class="app-button app-button--primary"
					onclick={() => save('maintenance')}
					disabled={saving === 'maintenance'}><Save size={16} />Save Maintenance</button
				>
			</footer>
		</section>
		<section class="settings-card">
			<header>
				<Cast />
				<div>
					<p class="eyebrow">Devices / Virtual Tuner</p>
					<h2>Virtual network tuners</h2>
				</div>
			</header>
			<div class="fields">
				<p class="section-note">
					Enable virtual tuners individually from <a href="/studio">Device outputs</a> on the Studio
					overview.
				</p>
				<label
					>Simultaneous tuners per lineup<input
						type="number"
						min="1"
						max="255"
						bind:value={settings.virtual_tuner.tuner_count}
					/>{#if errors.tuner_count}<em>{errors.tuner_count}</em>{/if}<small
						>Advertised capacity only; actual concurrency still depends on your source limits.</small
					></label
				>
			</div>
			<footer>
				<button
					class="app-button app-button--primary"
					onclick={() => save('devices')}
					disabled={saving === 'devices'}><Save size={16} />Save Devices</button
				>
			</footer>
		</section>
		<section class="settings-card theme-card">
			<header>
				<Palette />
				<div>
					<p class="eyebrow">Appearance</p>
					<h2>Theme</h2>
				</div>
			</header>
			<div class="theme-options">
				{#each themes as theme}<button
						class:active={preferences.theme === theme}
						onclick={() => setTheme(theme)}
						><span class={theme}></span><strong>{theme[0].toUpperCase()}{theme.slice(1)}</strong
						><small
							>{theme === 'system'
								? 'Follow the device in Studio; Watch stays cinematic.'
								: `Use ${theme} surfaces everywhere.`}</small
						></button
					>{/each}
			</div>
		</section>
	</div>{/if}

<style>
	.settings-message {
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
	.settings-message button {
		border: 0;
		background: transparent;
		text-decoration: underline;
		cursor: pointer;
	}
	.settings-grid,
	.settings-loading {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
		padding: 0 clamp(1.4rem, 3vw, 2.5rem) 3rem;
	}
	.settings-loading div {
		height: 28rem;
		border-radius: 1.2rem;
	}
	.settings-card {
		display: flex;
		overflow: hidden;
		flex-direction: column;
		border: 1px solid var(--line);
		border-radius: 1.2rem;
		background: var(--surface);
	}
	.settings-card > header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		border-bottom: 1px solid var(--line);
		padding: 1rem;
	}
	.settings-card > header > :global(svg) {
		box-sizing: content-box;
		padding: 0.7rem;
		border-radius: 0.8rem;
		background: color-mix(in oklch, var(--periwinkle) 15%, transparent);
		color: var(--periwinkle);
	}
	.settings-card h2 {
		margin: 0;
		font-size: 1.3rem;
	}
	.restart {
		display: inline-flex;
		border-radius: 99px;
		background: color-mix(in oklch, var(--sun) 20%, transparent);
		padding: 0.18rem 0.4rem;
		color: var(--sun);
		font-family: var(--font-sans);
		font-size: 0.55rem;
		font-weight: 800;
		letter-spacing: 0.04em;
		vertical-align: middle;
	}
	.fields {
		display: grid;
		gap: 0.8rem;
		padding: 1rem;
	}
	.section-note {
		margin: 0;
		border: 1px solid var(--line);
		border-radius: 0.75rem;
		background: var(--surface-raised);
		padding: 0.7rem;
		color: var(--muted);
		font-size: 0.67rem;
	}
	.section-note a {
		color: var(--periwinkle);
		font-weight: 750;
	}
	.fields.two {
		grid-template-columns: 1fr 1fr;
	}
	.field-pair {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.8rem;
	}
	.fields label {
		display: grid;
		gap: 0.3rem;
		color: var(--muted);
		font-size: 0.67rem;
		font-weight: 700;
	}
	.fields input {
		min-height: 2.7rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface-raised);
		padding: 0.5rem 0.65rem;
		color: var(--text);
	}
	.fields input:disabled {
		opacity: 0.55;
	}
	.fields label > em {
		color: var(--error);
		font-size: 0.6rem;
		font-style: normal;
	}
	.fields label > small {
		font-size: 0.6rem;
		font-weight: 500;
	}
	.switch-row {
		display: flex !important;
		min-height: 3.4rem;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		border: 1px solid var(--line);
		border-radius: 0.75rem;
		padding: 0.55rem 0.7rem;
	}
	.switch-row span {
		display: grid;
	}
	.switch-row strong {
		color: var(--text);
		font-size: 0.7rem;
	}
	.switch-row input {
		width: 2.8rem;
		height: 1.5rem;
		min-height: 0;
		accent-color: var(--periwinkle);
	}
	.settings-card > footer {
		display: flex;
		justify-content: flex-end;
		margin-top: auto;
		border-top: 1px solid var(--line);
		padding: 0.75rem 1rem;
	}
	.theme-card {
		grid-column: 1/3;
	}
	.theme-options {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.7rem;
		padding: 1rem;
	}
	.theme-options button {
		display: grid;
		grid-template-columns: auto 1fr;
		align-items: center;
		gap: 0.2rem 0.7rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		background: var(--surface-raised);
		padding: 0.7rem;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}
	.theme-options button.active {
		border-color: var(--periwinkle);
		box-shadow: inset 0 0 0 1px var(--periwinkle);
	}
	.theme-options button > span {
		grid-row: 1/3;
		width: 2.7rem;
		height: 2.7rem;
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		background: #10131a;
	}
	.theme-options button > span.light {
		background: #f7f7f2;
	}
	.theme-options button > span.system {
		background: linear-gradient(135deg, #f7f7f2 50%, #10131a 50%);
	}
	.theme-options small {
		color: var(--muted);
		font-size: 0.6rem;
	}
	.settings-error {
		display: grid;
		min-height: 20rem;
		place-items: center;
		align-content: center;
		gap: 1rem;
	}
	@media (max-width: 1000px) {
		.settings-grid,
		.settings-loading {
			grid-template-columns: 1fr;
		}
		.theme-card {
			grid-column: 1;
		}
	}
	@media (max-width: 600px) {
		.settings-grid,
		.settings-loading {
			padding-inline: 1rem;
		}
		.settings-message {
			margin-inline: 1rem;
		}
		.fields.two,
		.field-pair,
		.theme-options {
			grid-template-columns: 1fr;
		}
	}
</style>
