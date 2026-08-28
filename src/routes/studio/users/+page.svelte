<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { KeyRound, ShieldCheck, Trash2, UserPlus, Users } from '@lucide/svelte';
	import { api, XiviAPIError } from '$lib/api/client';
	import ReauthenticationDialog from '$lib/components/security/ReauthenticationDialog.svelte';
	import StudioHeader from '$lib/components/studio/StudioHeader.svelte';

	type User = {
		id: number;
		username: string;
		role: 'admin' | 'viewer';
		must_change_password: boolean;
		mfa_enabled: boolean;
		disabled_at?: string;
		created_at: string;
		lineup_ids: number[];
	};
	type Lineup = { id: number; name: string };
	type Response = { items: User[]; lineups: Lineup[] };
	type AuditEvent = {
		id: number;
		actor_user_id?: number;
		actor_username?: string;
		target_user_id?: number;
		target_username?: string;
		action: string;
		outcome: string;
		resource_type?: string;
		resource_id?: string;
		client_ip?: string;
		detail?: string;
		created_at: string;
	};
	type AuditResponse = { items: AuditEvent[]; next_cursor?: string; total: number };
	type MediaKey = {
		id: number;
		name: string;
		network_scope: 'public' | 'lan';
		created_at: string;
		last_used_at?: string;
		expires_at?: string;
		revoked_at?: string;
		lineup_ids: number[];
	};
	const client = useQueryClient();
	const query = createQuery(() => ({
		queryKey: ['studio', 'users'],
		queryFn: () => api<Response>('/api/v2/studio/users')
	}));
	const auditQuery = createQuery(() => ({
		queryKey: ['studio', 'security-audit'],
		queryFn: () => api<AuditResponse>('/api/v2/studio/security/audit?limit=50'),
		refetchInterval: 30_000
	}));
	let username = $state(''),
		password = $state(''),
		role = $state<'admin' | 'viewer'>('viewer'),
		lineupIDs = $state<number[]>([]);
	let message = $state(''),
		error = $state('');
	let reauthOpen = $state(false),
		reauthReason = $state('make this sensitive change'),
		pendingSensitiveAction = $state<(() => Promise<void>) | null>(null);
	$effect(() => {
		if (reauthOpen) return;
		pendingSensitiveAction = null;
	});
	let resetFor = $state<number | null>(null),
		resetPassword = $state('');
	let mediaKeys = $state<Record<number, MediaKey[]>>({});
	function errorMessage(caught: unknown) {
		return caught instanceof XiviAPIError
			? caught.detail.message
			: 'The request could not be completed.';
	}
	function auditActor(event: AuditEvent) {
		if (event.actor_username) return event.actor_username;
		if (event.action === 'login' && event.target_username) return event.target_username;
		if (event.actor_user_id) return 'Deleted user';
		if (event.action === 'login') return 'Unauthenticated request';
		return 'Xivi system';
	}
	function auditTarget(event: AuditEvent) {
		if (event.action === 'login') return '';
		if (event.target_username && event.target_username !== event.actor_username) {
			return event.target_username;
		}
		if (event.target_user_id && !event.target_username) return 'Deleted user';
		if (event.resource_type === 'user' && event.resource_id && !event.target_username) {
			return 'Deleted user';
		}
		return '';
	}
	function auditResource(event: AuditEvent) {
		if (!event.resource_type || event.resource_type === 'user') return '';
		return `${event.resource_type.replaceAll('_', ' ')}${event.resource_id ? ` ${event.resource_id}` : ''}`;
	}
	async function run(action: () => Promise<void>) {
		message = '';
		error = '';
		try {
			await action();
		} catch (caught) {
			error = errorMessage(caught);
		}
	}
	async function runSensitive(reason: string, action: () => Promise<void>) {
		message = '';
		error = '';
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
	function toggleCreate(id: number, checked: boolean) {
		lineupIDs = checked ? [...new Set([...lineupIDs, id])] : lineupIDs.filter((v) => v !== id);
	}
	async function create(event: SubmitEvent) {
		event.preventDefault();
		await runSensitive('create a user', async () => {
			await api('/api/v2/studio/users', {
				method: 'POST',
				body: JSON.stringify({
					username,
					password,
					role,
					disabled: false,
					lineup_ids: role === 'admin' ? [] : lineupIDs
				})
			});
			username = '';
			password = '';
			lineupIDs = [];
			await client.invalidateQueries({ queryKey: ['studio', 'users'] });
			message = 'User created with a temporary password.';
		});
	}
	async function update(
		user: User,
		changes: Partial<{ role: 'admin' | 'viewer'; disabled: boolean; lineup_ids: number[] }>
	) {
		await runSensitive(`update ${user.username}`, async () => {
			await api(`/api/v2/studio/users/${user.id}`, {
				method: 'PATCH',
				body: JSON.stringify({
					role: changes.role ?? user.role,
					disabled: changes.disabled ?? !!user.disabled_at,
					lineup_ids: changes.lineup_ids ?? user.lineup_ids
				})
			});
			await client.invalidateQueries({ queryKey: ['studio', 'users'] });
			message = `${user.username} updated; their sessions and device access were revoked.`;
		});
	}
	async function reset(user: User) {
		await runSensitive(`set a temporary password for ${user.username}`, async () => {
			await api(`/api/v2/studio/users/${user.id}/password-reset`, {
				method: 'POST',
				body: JSON.stringify({ password: resetPassword })
			});
			resetFor = null;
			resetPassword = '';
			message = `Temporary password set for ${user.username}; their sessions and device access were revoked.`;
		});
	}
	async function resetMFA(user: User) {
		await runSensitive(`reset MFA for ${user.username}`, async () => {
			await api(`/api/v2/studio/users/${user.id}/mfa`, { method: 'DELETE' });
			await client.invalidateQueries({ queryKey: ['studio', 'users'] });
			message = `MFA reset for ${user.username}.`;
		});
	}
	async function revokeSessions(user: User) {
		await runSensitive(`revoke every session for ${user.username}`, async () => {
			await api(`/api/v2/studio/users/${user.id}/sessions`, { method: 'DELETE' });
			message = `Every session for ${user.username} was revoked.`;
		});
	}
	async function revokeKeys(user: User) {
		await runSensitive(`revoke all device access for ${user.username}`, async () => {
			await api(`/api/v2/studio/users/${user.id}/media-keys`, { method: 'DELETE' });
			message = `All device access for ${user.username} was revoked.`;
		});
	}
	async function inspectKeys(user: User) {
		await run(async () => {
			const response = await api<{ items: MediaKey[] }>(
				`/api/v2/studio/users/${user.id}/media-keys`
			);
			mediaKeys = { ...mediaKeys, [user.id]: response.items };
		});
	}
	async function revokeKey(user: User, key: MediaKey) {
		await runSensitive(`revoke ${key.name} for ${user.username}`, async () => {
			await api(`/api/v2/studio/users/${user.id}/media-keys/${key.id}`, { method: 'DELETE' });
			await inspectKeys(user);
			message = `${key.name} was revoked.`;
		});
	}
	async function deleteUser(user: User) {
		if (!confirm(`Permanently delete ${user.username}?`)) return;
		await runSensitive(`delete ${user.username}`, async () => {
			await api(`/api/v2/studio/users/${user.id}`, { method: 'DELETE' });
			await client.invalidateQueries({ queryKey: ['studio', 'users'] });
			message = `${user.username} was deleted.`;
		});
	}
</script>

<svelte:head><title>Users · Xivi Studio</title></svelte:head>
<section class="users-page">
	<StudioHeader
		eyebrow="Security"
		title="Users"
		description="Create accounts, set roles, and grant viewers the smallest lineup access they need."
	/>
	{#if message}<p class="notice" role="status">{message}</p>{/if}{#if error}<p
			class="error"
			role="alert"
		>
			{error}
		</p>{/if}
	<div class="security-grid">
		<article>
			<div class="title">
				<UserPlus size={20} />
				<div>
					<h2>Create a user</h2>
					<p>There is no public registration.</p>
				</div>
			</div>
			<form onsubmit={create}>
				<label>Username<input bind:value={username} required maxlength="64" /></label><label
					>Temporary passphrase<input
						type="password"
						bind:value={password}
						required
						minlength="8"
						maxlength="128"
					/></label
				><label
					>Role<select bind:value={role}
						><option value="viewer">Viewer</option><option value="admin">Administrator</option
						></select
					></label
				>{#if role === 'viewer'}<fieldset>
						<legend>Lineup grants</legend>{#each query.data?.lineups ?? [] as lineup}<label
								class="check"
								><input
									type="checkbox"
									checked={lineupIDs.includes(lineup.id)}
									onchange={(e) => toggleCreate(lineup.id, e.currentTarget.checked)}
								/>{lineup.name}</label
							>{/each}
					</fieldset>{/if}<button class="app-button app-button--primary" type="submit"
					>Create user</button
				>
			</form>
		</article>
	</div>
	<div class="directory">
		<div class="directory-title">
			<Users size={20} />
			<h2>Account directory</h2>
			<span>{query.data?.items.length ?? 0}</span>
		</div>
		{#if query.isPending}<p class="empty">
				Loading users…
			</p>{:else}{#each query.data?.items ?? [] as user}<article
					class:disabled={!!user.disabled_at}
					class="user-card"
				>
					<div class="identity">
						<span class="avatar">{user.username.slice(0, 2).toUpperCase()}</span>
						<div>
							<strong>{user.username}</strong><span
								>{user.role} · {user.mfa_enabled
									? 'MFA enabled'
									: 'MFA off'}{user.must_change_password ? ' · password change required' : ''}</span
							>
						</div>
					</div>
					<div class="controls">
						<label
							>Role<select
								value={user.role}
								onchange={(e) =>
									update(user, { role: e.currentTarget.value as 'admin' | 'viewer' })}
								><option value="viewer">Viewer</option><option value="admin">Admin</option></select
							></label
						><label class="switch"
							><input
								type="checkbox"
								checked={!user.disabled_at}
								onchange={(e) => update(user, { disabled: !e.currentTarget.checked })}
							/><span>Enabled</span></label
						>
					</div>
					{#if user.role === 'viewer'}<fieldset>
							<legend>Can watch</legend>{#each query.data?.lineups ?? [] as lineup}<label
									class="check"
									><input
										type="checkbox"
										checked={user.lineup_ids.includes(lineup.id)}
										onchange={(e) =>
											update(user, {
												lineup_ids: e.currentTarget.checked
													? [...new Set([...user.lineup_ids, lineup.id])]
													: user.lineup_ids.filter((id) => id !== lineup.id)
											})}
									/>{lineup.name}</label
								>{/each}
						</fieldset>{/if}
					<div class="actions">
						{#if resetFor === user.id}<input
								aria-label={`Temporary password for ${user.username}`}
								type="password"
								bind:value={resetPassword}
								minlength="8"
								placeholder="New temporary passphrase"
							/><button class="app-button app-button--primary" onclick={() => reset(user)}
								>Set password</button
							><button class="app-button app-button--quiet" onclick={() => (resetFor = null)}
								>Cancel</button
							>{:else}<button
								class="app-button app-button--secondary"
								onclick={() => (resetFor = user.id)}><KeyRound size={17} /> Reset password</button
							>{/if}{#if user.mfa_enabled}<button
								class="app-button app-button--secondary"
								onclick={() => resetMFA(user)}>Reset MFA</button
							>{/if}<button
							class="app-button app-button--quiet"
							onclick={() => revokeSessions(user)}>Revoke sessions</button
						><button class="app-button app-button--quiet" onclick={() => inspectKeys(user)}
							>Inspect device access</button
						><button class="app-button app-button--quiet" onclick={() => revokeKeys(user)}
							>Revoke all device access</button
						><button class="app-button danger" onclick={() => deleteUser(user)}
							><Trash2 size={17} /> Delete</button
						>
					</div>
					{#if mediaKeys[user.id]}<div class="key-list">
							{#if !mediaKeys[user.id].length}<small>No device access.</small
								>{:else}{#each mediaKeys[user.id] as key}<div>
										<span
											><strong>{key.name}</strong><small
												>{key.network_scope} · {key.revoked_at
													? 'revoked'
													: key.last_used_at
														? `last used ${new Date(key.last_used_at).toLocaleString()}`
														: 'never used'}</small
											></span
										>{#if !key.revoked_at}<button
												class="app-button app-button--quiet"
												onclick={() => revokeKey(user, key)}>Revoke</button
											>{/if}
									</div>{/each}{/if}
						</div>{/if}
				</article>{/each}{/if}
	</div>
	<section class="audit">
		<div class="directory-title">
			<ShieldCheck size={20} />
			<h2>Security audit</h2>
			<span>{auditQuery.data?.total ?? 0}</span>
		</div>
		<div class="audit-list">
			{#if auditQuery.isPending}<p class="empty">
					Loading security events…
				</p>{:else if !auditQuery.data?.items.length}<p class="empty">
					No security events have been recorded.
				</p>{:else}{#each auditQuery.data.items as event}<article
						class:failure={event.outcome === 'failure' || event.outcome === 'denied'}
					>
						<time>{new Date(event.created_at).toLocaleString()}</time>
						<div class="audit-action">
							<strong>{event.action.replaceAll('_', ' ')}</strong>
							<span class="audit-outcome">{event.outcome}</span>
						</div>
						<div class="audit-identity">
							<strong>{auditActor(event)}</strong>
							{#if auditTarget(event)}<span>on {auditTarget(event)}</span>{/if}
							{#if auditResource(event)}<span>{auditResource(event)}</span>{/if}
						</div>
						<code>{event.client_ip ?? 'unknown client'}</code>{#if event.detail}<small
								>{event.detail.replaceAll('_', ' ')}</small
							>{/if}
					</article>{/each}{/if}
		</div>
	</section>
</section>

<ReauthenticationDialog
	bind:open={reauthOpen}
	reason={reauthReason}
	onconfirmed={continueSensitiveAction}
/>

<style>
	.users-page {
		padding: clamp(1rem, 3vw, 2.4rem);
		display: grid;
		gap: 1rem;
	}
	.notice,
	.error {
		margin: 0;
		border-radius: 0.8rem;
		padding: 0.75rem 1rem;
		font-size: 0.78rem;
	}
	.notice {
		background: color-mix(in oklch, var(--aqua) 20%, var(--surface));
	}
	.error {
		background: color-mix(in oklch, var(--error) 25%, var(--surface));
	}
	.security-grid {
		display: grid;
		grid-template-columns: minmax(0, 44rem);
		gap: 1rem;
		align-items: start;
	}
	article,
	.directory,
	.audit {
		border: 1px solid var(--line);
		border-radius: 1.1rem;
		background: var(--surface);
		padding: 1rem;
		display: grid;
		gap: 0.8rem;
	}
	.title,
	.directory-title,
	.identity {
		display: flex;
		align-items: center;
		gap: 0.7rem;
	}
	.title :global(svg) {
		color: var(--aqua);
	}
	h2,
	p {
		margin: 0;
	}
	.title p {
		color: var(--muted);
		font-size: 0.72rem;
	}
	form {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.65rem;
	}
	form fieldset,
	form button {
		grid-column: 1/-1;
	}
	label {
		display: grid;
		gap: 0.3rem;
		color: var(--muted);
		font-size: 0.7rem;
		font-weight: 800;
	}
	label span {
		font-weight: 500;
	}
	input,
	select {
		min-height: 2.7rem;
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		background: var(--deep);
		color: var(--text);
		padding: 0.55rem 0.7rem;
	}
	fieldset {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
		gap: 0.35rem;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		padding: 0.7rem;
	}
	legend {
		color: var(--muted);
		font-size: 0.68rem;
		font-weight: 800;
	}
	.check {
		grid-template-columns: auto 1fr;
		align-items: center;
	}
	.check input,
	.switch input {
		min-height: 1rem;
		width: 1rem;
	}
	.directory {
		gap: 0;
	}
	.directory-title {
		padding: 0.2rem 0.2rem 0.8rem;
	}
	.directory-title h2 {
		flex: 1;
	}
	.directory-title span {
		border-radius: 1rem;
		background: var(--surface-raised);
		padding: 0.25rem 0.55rem;
		font-size: 0.7rem;
	}
	.user-card {
		border-width: 1px 0 0;
		border-radius: 0;
		background: transparent;
		padding: 1rem 0.2rem;
	}
	.user-card.disabled {
		opacity: 0.58;
	}
	.identity {
		min-width: 0;
	}
	.identity > div {
		display: grid;
	}
	.identity span {
		color: var(--muted);
		font-size: 0.7rem;
	}
	.avatar {
		display: grid;
		width: 2.7rem;
		aspect-ratio: 1;
		place-items: center;
		border-radius: 0.8rem;
		background: var(--periwinkle);
		color: #fff !important;
		font-weight: 850;
	}
	.controls {
		display: flex;
		align-items: end;
		gap: 1rem;
	}
	.switch {
		display: flex;
		min-height: 2.7rem;
		align-items: center;
		gap: 0.45rem;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.actions input {
		flex: 1;
		min-width: 15rem;
	}
	.key-list {
		display: grid;
		border: 1px solid var(--line);
		border-radius: 0.8rem;
		padding: 0.45rem;
	}
	.key-list > div {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		padding: 0.45rem;
	}
	.key-list > div + div {
		border-top: 1px solid var(--line);
	}
	.key-list span {
		display: grid;
		flex: 1;
	}
	.key-list small {
		color: var(--muted);
	}
	.danger {
		background: color-mix(in oklch, var(--error) 24%, var(--surface));
		border-color: var(--error);
	}
	.empty {
		padding: 2rem;
		color: var(--muted);
		text-align: center;
	}
	.audit {
		gap: 0;
	}
	.audit-list {
		max-height: 26rem;
		overflow: auto;
		overscroll-behavior: auto;
	}
	.audit-list article {
		grid-template-columns: 10rem minmax(10rem, 1fr) minmax(10rem, 1fr) auto;
		border-width: 1px 0 0;
		border-radius: 0;
		background: transparent;
		padding: 0.7rem 0.2rem;
		font-size: 0.72rem;
	}
	.audit-list article.failure {
		border-left: 3px solid var(--error);
		padding-left: 0.6rem;
	}
	.audit-list time,
	.audit-list span,
	.audit-list small {
		color: var(--muted);
	}
	.audit-action,
	.audit-identity {
		display: grid;
		align-content: start;
		gap: 0.18rem;
		min-width: 0;
	}
	.audit-action strong,
	.audit-identity strong {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.audit-outcome {
		text-transform: capitalize;
	}
	.audit-list small {
		grid-column: 2/-1;
	}
	.audit-list code {
		font-size: 0.65rem;
		color: var(--muted);
	}
	@media (max-width: 800px) {
		.security-grid {
			grid-template-columns: 1fr;
		}
		form {
			grid-template-columns: 1fr;
		}
		form fieldset,
		form button {
			grid-column: auto;
		}
		.controls {
			flex-wrap: wrap;
		}
		.audit-list article {
			grid-template-columns: 1fr 1fr;
		}
		.audit-list small {
			grid-column: 1/-1;
		}
	}
</style>
