<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import {
		Copy,
		KeyRound,
		LockKeyhole,
		LogOut,
		ShieldCheck,
		Trash2,
		UserRound
	} from '@lucide/svelte';
	import qrcode from 'qrcode-generator';
	import { api, XiviAPIError } from '$lib/api/client';
	import type { LineupSummary } from '$lib/api/types';
	import ReauthenticationDialog from '$lib/components/security/ReauthenticationDialog.svelte';
	import {
		auth,
		clearSession,
		loadSession,
		setSession,
		type SessionPrincipal
	} from '$lib/state/auth.svelte';
	import { copyText } from '$lib/browser/clipboard';

	type MediaKey = {
		id: number;
		name: string;
		network_scope: 'public' | 'lan';
		created_at: string;
		expires_at?: string;
		last_used_at?: string;
		last_used_ip?: string;
		revoked_at?: string;
		lineup_ids: number[];
		links?: Record<string, Record<string, string>>;
	};
	type Session = {
		id: number;
		transport_scope: string;
		created_at: string;
		last_seen_at: string;
		absolute_expires_at: string;
		revoked_at?: string;
		client_ip: string;
	};
	type MediaKeyList = {
		items: MediaKey[];
		public_https_available: boolean;
	};

	const client = useQueryClient();
	const lineups = createQuery(() => ({
		queryKey: ['watch', 'lineups'],
		queryFn: () => api<{ items: LineupSummary[] }>('/api/v2/watch/lineups'),
		enabled: !auth.principal?.must_change_password
	}));
	const keys = createQuery(() => ({
		queryKey: ['account', 'media-keys'],
		queryFn: () => api<MediaKeyList>('/api/v2/account/media-keys'),
		enabled: !auth.principal?.must_change_password
	}));
	const sessions = createQuery(() => ({
		queryKey: ['account', 'sessions'],
		queryFn: () => api<{ items: Session[] }>('/api/v2/auth/sessions'),
		enabled: !auth.principal?.must_change_password
	}));

	let currentPassword = $state('');
	let newPassword = $state('');
	let newPasswordConfirmation = $state('');
	let passwordMFA = $state('');
	let reauthOpen = $state(false);
	let reauthReason = $state('this sensitive change');
	let pendingSensitiveAction = $state<(() => Promise<void>) | null>(null);
	$effect(() => {
		if (reauthOpen) return;
		pendingSensitiveAction = null;
	});
	let status = $state('');
	let error = $state('');
	let enrollment = $state<{ secret: string; otpauth_uri: string } | null>(null);
	let enrollmentQRCode = $state('');
	let confirmCode = $state('');
	let recoveryCodes = $state<string[]>([]);
	let keyName = $state('Living room player');
	let keyScope = $state<'public' | 'lan'>('lan');
	let keyScopeSelected = $state(false);
	let keyLineups = $state<number[]>([]);
	$effect(() => {
		if (!keyScopeSelected && keys.data) {
			keyScope = keys.data.public_https_available ? 'public' : 'lan';
			keyScopeSelected = true;
		}
	});

	async function perform(action: () => Promise<void>) {
		error = '';
		status = '';
		try {
			await action();
		} catch (caught) {
			error =
				caught instanceof XiviAPIError
					? caught.detail.message
					: 'The request could not be completed.';
		}
	}
	function errorMessage(caught: unknown) {
		return caught instanceof XiviAPIError
			? caught.detail.message
			: 'The request could not be completed.';
	}
	async function performSensitive(reason: string, action: () => Promise<void>) {
		error = '';
		status = '';
		try {
			await action();
		} catch (caught) {
			if (caught instanceof XiviAPIError && caught.detail.code === 'reauthentication_required') {
				pendingSensitiveAction = action;
				reauthReason = reason;
				reauthOpen = true;
				return;
			}
			error = errorMessage(caught);
		}
	}
	async function continueSensitiveAction() {
		if (!pendingSensitiveAction) return;
		const action = pendingSensitiveAction;
		await action();
		pendingSensitiveAction = null;
	}
	async function changePassword(event: SubmitEvent) {
		event.preventDefault();
		if (newPassword !== newPasswordConfirmation) {
			status = '';
			error = 'The new passphrases do not match.';
			return;
		}
		await perform(async () => {
			const principal = await api<SessionPrincipal>('/api/v2/auth/password', {
				method: 'POST',
				body: JSON.stringify({
					current_password: currentPassword,
					new_password: newPassword,
					mfa_code: passwordMFA
				})
			});
			setSession(principal);
			currentPassword = '';
			newPassword = '';
			newPasswordConfirmation = '';
			passwordMFA = '';
			status = 'Password changed and other sessions were revoked.';
		});
	}
	async function startMFA() {
		await performSensitive('set up authenticator MFA', async () => {
			const response = await api<{ secret: string; otpauth_uri: string }>(
				'/api/v2/auth/mfa/enroll',
				{ method: 'POST' }
			);
			enrollment = response;
			try {
				const code = qrcode(0, 'M');
				code.addData(response.otpauth_uri);
				code.make();
				enrollmentQRCode = code.createDataURL(8, 16);
			} catch {
				enrollmentQRCode = '';
			}
		});
	}
	async function confirmMFA() {
		if (!enrollment) return;
		await performSensitive('enable authenticator MFA', async () => {
			const response = await api<{ recovery_codes: string[] }>('/api/v2/auth/mfa/confirm', {
				method: 'POST',
				body: JSON.stringify({ secret: enrollment?.secret, code: confirmCode })
			});
			recoveryCodes = response.recovery_codes;
			enrollment = null;
			enrollmentQRCode = '';
			confirmCode = '';
			await loadSession();
			status = 'Authenticator protection is enabled. Save the recovery codes now.';
		});
	}
	async function disableMFA() {
		await performSensitive('disable authenticator MFA', async () => {
			await api('/api/v2/auth/mfa', { method: 'DELETE' });
			clearSession();
			await goto('/login', { replaceState: true });
		});
	}
	async function regenerateRecoveryCodes() {
		await performSensitive('replace your recovery codes', async () => {
			const response = await api<{ recovery_codes: string[] }>('/api/v2/auth/mfa/recovery-codes', {
				method: 'POST'
			});
			recoveryCodes = response.recovery_codes;
			status = 'New recovery codes created. Every older recovery code is now invalid.';
		});
	}
	async function revokeSession(id: number) {
		await perform(async () => {
			await api(`/api/v2/auth/sessions/${id}`, { method: 'DELETE' });
			await client.invalidateQueries({ queryKey: ['account', 'sessions'] });
			status = 'Session revoked.';
		});
	}
	async function createKey(event: SubmitEvent) {
		event.preventDefault();
		await performSensitive('add device access', async () => {
			await api<{ key: MediaKey }>('/api/v2/account/media-keys', {
				method: 'POST',
				body: JSON.stringify({ name: keyName, network_scope: keyScope, lineup_ids: keyLineups })
			});
			await client.invalidateQueries({ queryKey: ['account', 'media-keys'] });
			status = 'Device access added. Its M3U and XMLTV links remain available below.';
		});
	}
	async function revokeKey(id: number) {
		await perform(async () => {
			await api(`/api/v2/account/media-keys/${id}`, { method: 'DELETE' });
			await client.invalidateQueries({ queryKey: ['account', 'media-keys'] });
			status = 'Device access revoked.';
		});
	}
	async function logout() {
		error = '';
		status = '';
		try {
			await api('/api/v2/auth/logout', { method: 'POST' });
			client.clear();
			clearSession();
			await goto('/login', { replaceState: true });
		} catch (caught) {
			// A 401 means middleware already confirmed that the session is gone.
			// For CSRF, network, or server failures, remain on the page and expose
			// the failure instead of pretending that navigation ended the session.
			if (!auth.principal) {
				client.clear();
				await goto('/login', { replaceState: true });
				return;
			}
			error = errorMessage(caught);
		}
	}
	function toggleLineup(id: number, checked: boolean) {
		keyLineups = checked
			? [...new Set([...keyLineups, id])]
			: keyLineups.filter((value) => value !== id);
	}
	function lineupName(id: string) {
		return lineups.data?.items.find((lineup) => lineup.id === Number(id))?.name ?? `Lineup ${id}`;
	}
	function outputLinkLabel(kind: string) {
		const [network, format] = kind.split('_');
		return `Copy ${network === 'public' ? 'public' : 'local'} ${format?.toUpperCase() ?? 'link'}`;
	}
	function orderedOutputLinks(outputLinks: Record<string, string>) {
		const order = ['public_m3u', 'public_xmltv', 'local_m3u', 'local_xmltv'];
		return order.flatMap((kind) => (outputLinks[kind] ? [[kind, outputLinks[kind]] as const] : []));
	}
	async function copy(value: string) {
		await perform(async () => {
			await copyText(value);
			status = 'Copied.';
		});
	}
</script>

<svelte:head><title>Account security · Xivi</title></svelte:head>

<section class="account-page">
	<header>
		<div>
			<p class="eyebrow">Account & security</p>
			<h1>{auth.principal?.username}</h1>
			{#if auth.principal?.must_change_password}
				<p>Administrator · Initial security setup</p>
			{:else}
				<p>
					{auth.principal?.role === 'admin' ? 'Administrator' : 'Viewer'} · {auth.principal
						?.lineup_ids.length ?? 0} explicit lineup grants
				</p>
			{/if}
		</div>
		<button class="app-button app-button--secondary" onclick={logout}
			><LogOut size={18} /> Sign out</button
		>
	</header>
	{#if auth.principal?.must_change_password}<div class="required" role="alert">
			<LockKeyhole size={20} /><span
				><strong>Change the temporary password now.</strong> Watch and Studio remain locked until you
				choose your own passphrase.</span
			>
		</div>{/if}
	{#if auth.principal?.role === 'admin' && !auth.principal.mfa_enabled && !auth.principal.must_change_password}<div
			class="warning"
			role="alert"
		>
			<ShieldCheck size={20} /><span
				><strong>Your administrator account does not use MFA.</strong> Enable an authenticator before
				exposing Xivi to the internet.</span
			>
		</div>{/if}
	{#if status}<p class="notice" role="status">{status}</p>{/if}
	{#if error}<p class="error" role="alert">{error}</p>{/if}

	<div class:password-only={auth.principal?.must_change_password} class="account-grid">
		<article class="password-card">
			<div class="card-title">
				<LockKeyhole size={20} />
				<div>
					<h2>Password</h2>
					<p>8–128 characters; passphrases are welcome.</p>
				</div>
			</div>
			<form onsubmit={changePassword}>
				<label
					>Current password<input
						type="password"
						bind:value={currentPassword}
						autocomplete="current-password"
						required
					/></label
				>
				<label
					>New passphrase<input
						type="password"
						bind:value={newPassword}
						autocomplete="new-password"
						minlength="8"
						maxlength="128"
						required
					/></label
				>
				<label
					>Confirm new passphrase<input
						bind:value={newPasswordConfirmation}
						type="password"
						autocomplete="new-password"
						minlength="8"
						maxlength="128"
						required
					/></label
				>
				{#if auth.principal?.mfa_enabled}<label
						>Authenticator or recovery code<input
							bind:value={passwordMFA}
							autocomplete="one-time-code"
							required
						/></label
					>{/if}
				<button class="app-button app-button--primary" type="submit">Change password</button>
			</form>
		</article>

		{#if !auth.principal?.must_change_password}
			<article id="multi-factor" class="mfa-card">
				<div class="card-title">
					<ShieldCheck size={20} />
					<div>
						<h2>Authenticator MFA</h2>
						<p>Protect interactive sign-in with a time-based code.</p>
					</div>
				</div>
				{#if recoveryCodes.length}
					<div class="recovery">
						<strong>Save these one-time recovery codes</strong>
						<div>
							{#each recoveryCodes as code}<code>{code}</code>{/each}
						</div>
						<button
							class="app-button app-button--secondary"
							onclick={() => copy(recoveryCodes.join('\n'))}><Copy size={17} /> Copy codes</button
						>
					</div>
				{:else if enrollment}
					<div class="enroll">
						<div class="mfa-qr">
							{#if enrollmentQRCode}
								<div class="mfa-qr-frame">
									<img
										src={enrollmentQRCode}
										alt="QR code for adding Xivi to an authenticator app"
									/>
								</div>
								<strong>Scan with your authenticator app</strong>
								<span>Then enter the current six-digit code below.</span>
							{:else}
								<p>The QR code could not be generated. Use the manual setup key below.</p>
							{/if}
						</div>
						<details class="mfa-manual">
							<summary>Enter a setup key manually</summary>
							<code>{enrollment.secret}</code>
							<a class="app-button app-button--secondary" href={enrollment.otpauth_uri}
								>Open in authenticator</a
							>
						</details>
						<label
							>Six-digit code<input
								bind:value={confirmCode}
								inputmode="numeric"
								autocomplete="one-time-code"
							/></label
						><button class="app-button app-button--primary" onclick={confirmMFA}
							>Confirm and enable</button
						>
					</div>
				{:else if auth.principal?.mfa_enabled}
					<div class="enabled">
						<ShieldCheck size={22} /><strong>MFA is enabled</strong><span
							>Disabling it revokes every session.</span
						>
					</div>
					<button class="app-button app-button--secondary" onclick={regenerateRecoveryCodes}
						>Replace recovery codes</button
					><button class="app-button danger" onclick={disableMFA}>Disable MFA</button>
				{:else}
					<button class="app-button app-button--primary" onclick={startMFA}>Start enrollment</button
					>
				{/if}
			</article>

			<article class="sessions-card">
				<div class="card-title">
					<UserRound size={20} />
					<div>
						<h2>Signed-in sessions</h2>
						<p>Revoke browsers or devices you no longer recognize.</p>
					</div>
				</div>
				<div class="session-list">
					{#each sessions.data?.items ?? [] as session}<div
							class:revoked={!!session.revoked_at}
							class="session-row"
						>
							<div>
								<strong
									>{session.transport_scope === 'https' ? 'Public HTTPS' : 'Direct LAN'}</strong
								><span
									>{session.client_ip} · Last seen {new Date(
										session.last_seen_at
									).toLocaleString()}</span
								>
							</div>
							{#if !session.revoked_at}<button
									class="app-button app-button--quiet"
									onclick={() => revokeSession(session.id)}>Revoke</button
								>{:else}<span>Revoked</span>{/if}
						</div>{/each}
				</div>
			</article>

			<article class="keys-card">
				<div class="card-title">
					<KeyRound size={20} />
					<div>
						<h2>Device access</h2>
						<p>
							Reusable M3U and XMLTV links for TVs and player apps. The underlying credential stays
							hidden and each device can be revoked independently.
						</p>
					</div>
				</div>
				<form class="key-form" onsubmit={createKey}>
					<label>Device name<input bind:value={keyName} maxlength="80" required /></label>
					<label
						>Network scope<select
							value={keyScope}
							onchange={(event) => {
								keyScope = event.currentTarget.value as 'public' | 'lan';
								keyScopeSelected = true;
							}}
							>{#if keys.data?.public_https_available}<option value="public"
									>Public HTTPS + local network</option
								>{/if}<option value="lan">Local network only</option></select
						></label
					>
					{#if keys.data && !keys.data.public_https_available}<p class="scope-note">
							Public copy links become available after an administrator configures the Public HTTPS
							base URL and restarts Xivi.
						</p>{/if}
					<fieldset>
						<legend>Allowed lineups</legend>{#each lineups.data?.items ?? [] as lineup}<label
								class="check"
								><input
									type="checkbox"
									checked={keyLineups.includes(lineup.id)}
									onchange={(event) => toggleLineup(lineup.id, event.currentTarget.checked)}
								/>{lineup.name}</label
							>{/each}
					</fieldset>
					<button class="app-button app-button--primary" type="submit" disabled={!keyLineups.length}
						>Add device access</button
					>
				</form>
				<div class="key-list">
					{#each keys.data?.items ?? [] as key}
						<div class:revoked={!!key.revoked_at} class="key-row">
							<div class="key-summary">
								<strong>{key.name}</strong>
								<span
									>{key.network_scope === 'public'
										? 'Public HTTPS + local network'
										: 'Local network only'} · {key.lineup_ids.length} lineup{key.lineup_ids
										.length === 1
										? ''
										: 's'}</span
								>
								<small
									>{key.last_used_at
										? `Last used ${new Date(key.last_used_at).toLocaleString()} from ${key.last_used_ip}`
										: 'Never used'}</small
								>
							</div>
							{#if !key.revoked_at}<button
									aria-label={`Revoke ${key.name}`}
									onclick={() => revokeKey(key.id)}><Trash2 size={17} /></button
								>{:else}<span>Revoked</span>{/if}
							{#if !key.revoked_at && Object.keys(key.links ?? {}).length}
								<div class="key-links">
									{#each Object.entries(key.links ?? {}) as [lineupID, outputLinks]}
										<div class="lineup-links">
											<strong>{lineupName(lineupID)}</strong>
											<div>
												{#each orderedOutputLinks(outputLinks) as [kind, link]}
													<button class="link-copy" onclick={() => copy(link)}
														><Copy size={15} /><span>{outputLinkLabel(kind)}</span></button
													>
												{/each}
											</div>
										</div>
									{/each}
								</div>
								{#if key.network_scope === 'lan' && keys.data?.public_https_available}<small
										class="scope-help"
										>This key is local-only. Add Public HTTPS device access to create public M3U and
										XMLTV links.</small
									>{/if}
							{:else if !key.revoked_at}
								<p class="legacy-links">
									Reusable links are unavailable for this older credential. Add replacement device
									access, update the player, then revoke this entry.
								</p>
							{/if}
						</div>
					{/each}
				</div>
			</article>
		{/if}
	</div>
</section>

<ReauthenticationDialog
	bind:open={reauthOpen}
	reason={reauthReason}
	onconfirmed={continueSensitiveAction}
/>

<style>
	.account-page {
		width: min(76rem, calc(100% - 2rem));
		margin: 0 auto;
		padding: clamp(1.3rem, 4vw, 3.5rem) 0 6rem;
	}
	header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}
	h1 {
		margin: 0.1rem 0;
		font-size: clamp(2.3rem, 5vw, 4.5rem);
	}
	header p {
		margin: 0;
		color: var(--muted);
	}
	.eyebrow {
		color: var(--aqua) !important;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		font-weight: 850;
		font-size: 0.7rem;
	}
	.required,
	.warning,
	.notice,
	.error {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		border-radius: 1rem;
		padding: 0.85rem 1rem;
		margin: 0 0 0.8rem;
		font-size: 0.8rem;
	}
	.required {
		background: var(--sun);
		color: var(--ink);
	}
	.warning {
		background: color-mix(in oklch, var(--coral) 75%, var(--surface));
		color: var(--ink);
	}
	.notice {
		background: color-mix(in oklch, var(--aqua) 22%, var(--surface));
	}
	.error {
		background: color-mix(in oklch, var(--error) 25%, var(--surface));
	}
	.account-grid {
		display: grid;
		grid-template-columns: minmax(19rem, 0.82fr) minmax(28rem, 1.18fr);
		grid-template-areas:
			'password sessions'
			'mfa sessions'
			'keys keys';
		align-items: start;
		gap: 1rem;
	}
	.account-grid.password-only {
		grid-template-columns: minmax(0, 32rem);
		grid-template-areas: 'password';
		justify-content: center;
	}
	.password-card {
		grid-area: password;
	}
	.mfa-card {
		grid-area: mfa;
	}
	.sessions-card {
		grid-area: sessions;
		align-self: stretch;
		align-content: start;
	}
	article {
		display: grid;
		gap: 1rem;
		border: 1px solid var(--line);
		border-radius: 1.25rem;
		background: var(--surface);
		padding: 1.2rem;
	}
	.keys-card {
		grid-area: keys;
	}
	.card-title {
		display: flex;
		gap: 0.75rem;
		align-items: flex-start;
	}
	.card-title :global(svg) {
		flex: none;
		color: var(--aqua);
	}
	.card-title h2,
	.card-title p {
		margin: 0;
	}
	.card-title p {
		color: var(--muted);
		font-size: 0.75rem;
	}
	form,
	.enroll,
	.recovery {
		display: grid;
		gap: 0.8rem;
	}
	label {
		display: grid;
		gap: 0.35rem;
		color: var(--muted);
		font-size: 0.72rem;
		font-weight: 800;
	}
	input,
	select {
		min-height: 2.8rem;
		width: 100%;
		border: 1px solid var(--line);
		border-radius: 0.75rem;
		background: var(--deep);
		padding: 0.6rem 0.75rem;
		color: var(--text);
	}
	fieldset {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
		gap: 0.4rem;
		border: 1px solid var(--line);
		border-radius: 0.85rem;
		padding: 0.8rem;
	}
	legend {
		color: var(--muted);
		font-size: 0.7rem;
		font-weight: 800;
	}
	.check {
		grid-template-columns: auto 1fr;
		align-items: center;
	}
	.check input {
		width: 1.1rem;
		min-height: 1.1rem;
	}
	.danger {
		background: color-mix(in oklch, var(--error) 24%, var(--surface));
		border-color: var(--error);
	}
	.enabled {
		display: grid;
		place-items: center;
		gap: 0.2rem;
		border-radius: 0.9rem;
		background: color-mix(in oklch, var(--aqua) 14%, transparent);
		padding: 1.2rem;
	}
	.enabled span {
		color: var(--muted);
		font-size: 0.72rem;
	}
	.enroll code {
		overflow-wrap: anywhere;
		border-radius: 0.7rem;
		background: var(--deep);
		padding: 0.75rem;
		color: var(--sun);
	}
	.mfa-qr {
		display: grid;
		justify-items: center;
		gap: 0.45rem;
		border-radius: 1rem;
		background: color-mix(in oklch, var(--aqua) 10%, var(--deep));
		padding: 1rem;
		text-align: center;
	}
	.mfa-qr-frame {
		width: min(16rem, 100%);
		border-radius: 0.9rem;
		background: #fff;
		padding: 0.65rem;
		box-shadow: 0 12px 28px rgb(0 0 0 / 0.2);
	}
	.mfa-qr img {
		display: block;
		width: 100%;
		height: auto;
	}
	.mfa-qr strong {
		margin-top: 0.25rem;
	}
	.mfa-qr span,
	.mfa-qr p {
		margin: 0;
		color: var(--muted);
		font-size: 0.72rem;
	}
	.mfa-manual {
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		padding: 0.75rem;
	}
	.mfa-manual summary {
		color: var(--muted);
		font-size: 0.75rem;
		font-weight: 800;
		cursor: pointer;
	}
	.mfa-manual code {
		display: block;
		margin-top: 0.7rem;
	}
	.mfa-manual a {
		margin-top: 0.55rem;
	}
	.recovery > div {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.35rem;
	}
	.recovery code {
		background: var(--deep);
		padding: 0.45rem;
		border-radius: 0.4rem;
		text-align: center;
	}
	.key-form {
		grid-template-columns: 1fr 1fr;
	}
	.key-form fieldset,
	.key-form button,
	.scope-note {
		grid-column: 1/-1;
	}
	.scope-note,
	.scope-help {
		margin: 0;
		color: var(--muted);
		font-size: 0.7rem;
		line-height: 1.45;
	}
	.link-copy {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		min-height: 2.6rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--surface-raised);
		padding: 0.5rem 0.7rem;
		color: var(--text);
		cursor: pointer;
		text-transform: capitalize;
	}
	.key-list {
		display: grid;
		gap: 0.5rem;
	}
	.key-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.8rem;
		border-top: 1px solid var(--line);
		padding: 0.8rem 0.2rem 0;
	}
	.key-summary {
		display: grid;
		min-width: 0;
	}
	.key-row span,
	.key-row small {
		color: var(--muted);
		font-size: 0.7rem;
	}
	.key-row button {
		display: grid;
		width: 2.6rem;
		height: 2.6rem;
		place-items: center;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: transparent;
		cursor: pointer;
	}
	.key-links,
	.legacy-links,
	.scope-help {
		grid-column: 1/-1;
	}
	.key-links {
		display: grid;
		gap: 0.55rem;
		border-radius: 0.85rem;
		background: color-mix(in oklch, var(--aqua) 7%, var(--deep));
		padding: 0.75rem;
	}
	.lineup-links {
		display: grid;
		gap: 0.45rem;
	}
	.lineup-links > strong {
		font-size: 0.75rem;
	}
	.lineup-links > div {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.lineup-links .link-copy {
		display: flex;
		width: auto;
		height: auto;
		background: var(--surface-raised);
	}
	.legacy-links {
		margin: 0;
		border-radius: 0.7rem;
		background: color-mix(in oklch, var(--sun) 9%, transparent);
		padding: 0.65rem 0.75rem;
		color: var(--muted);
		font-size: 0.72rem;
	}
	.key-row.revoked {
		opacity: 0.55;
	}
	.session-list {
		display: grid;
		max-height: 32rem;
		overflow-y: auto;
		padding-right: 0.25rem;
		scrollbar-gutter: stable;
	}
	.session-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.7rem;
		border-top: 1px solid var(--line);
		padding: 0.7rem 0;
	}
	.session-row > div {
		display: grid;
	}
	.session-row span {
		color: var(--muted);
		font-size: 0.68rem;
	}
	.session-row.revoked {
		opacity: 0.5;
	}
	@media (max-width: 760px) {
		.account-grid {
			grid-template-columns: 1fr;
			grid-template-areas:
				'password'
				'sessions'
				'mfa'
				'keys';
		}
		.keys-card {
			grid-area: keys;
		}
		.key-form {
			grid-template-columns: 1fr;
		}
		.key-form fieldset,
		.key-form button,
		.scope-note {
			grid-column: auto;
		}
		header {
			align-items: center;
		}
		header h1 {
			font-size: 2.4rem;
		}
	}
</style>
