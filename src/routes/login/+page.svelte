<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { LockKeyhole } from '@lucide/svelte';
	import SignalMark from '$lib/components/brand/SignalMark.svelte';
	import { api, XiviAPIError } from '$lib/api/client';
	import { auth, setSession, type SessionPrincipal } from '$lib/state/auth.svelte';

	let username = $state('');
	let password = $state('');
	let mfaCode = $state('');
	let submitting = $state(false);
	let error = $state('');
	$effect(() => {
		if (auth.initialPasswordChangeRequired && !username) username = 'xivi';
	});

	async function login(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = '';
		try {
			const principal = await api<SessionPrincipal>('/api/v2/auth/login', {
				method: 'POST',
				body: JSON.stringify({ username, password, mfa_code: mfaCode })
			});
			setSession(principal);
			const requested = page.url.searchParams.get('next') ?? '/';
			const safeNext =
				requested.startsWith('/') && !requested.startsWith('//') && !requested.includes('\\')
					? requested
					: '/';
			const adminOnlyNext =
				safeNext.startsWith('/studio') ||
				safeNext.startsWith('/docs/') ||
				safeNext.startsWith('/swagger');
			const next = principal.role === 'viewer' && adminOnlyNext ? '/' : safeNext;
			if (principal.must_change_password) {
				await goto('/account?change-password=required', { replaceState: true });
				return;
			}
			if (next.startsWith('/docs/') || next.startsWith('/swagger')) {
				window.location.assign(next);
				return;
			}
			await goto(next, { replaceState: true });
		} catch (caught) {
			error =
				caught instanceof XiviAPIError
					? caught.detail.message
					: 'Xivi could not reach the authentication service.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head><title>Sign in · Xivi</title></svelte:head>

<main class="login-page">
	<section class="login-story" aria-label="Xivi">
		<SignalMark />
		<div class="story-copy">
			<h1>Live TV,<br /><em>for your people.</em></h1>
			<p>Watch your channels. Organize the signal in Studio.</p>
		</div>
	</section>

	<section class="login-panel">
		<form onsubmit={login}>
			<div class="form-heading">
				<div class="lock-tile"><LockKeyhole size={23} /></div>
				<div>
					<p>Welcome back</p>
					<h2>Sign in to Xivi</h2>
				</div>
			</div>
			{#if auth.bootstrapRequired}
				<div class="bootstrap" role="alert">
					<strong>First-run account is still initializing</strong>
					Restart Xivi if this message remains visible.
				</div>
			{:else if auth.initialPasswordChangeRequired}
				<div class="bootstrap" role="alert">
					<strong>First sign-in</strong>
					{#if auth.initialLoginAllowed}
						<span
							>Use <code>xivi</code> / <code>xivi</code>. You will immediately choose a private
							passphrase.</span
						>
					{:else}
						<span
							>Open Xivi through its configured public HTTPS or trusted-LAN address to replace the
							temporary password.</span
						>
					{/if}
				</div>
			{/if}
			<label
				>Username<input
					bind:value={username}
					autocomplete="username"
					required
					maxlength="64"
				/></label
			>
			<label
				>Password<input
					bind:value={password}
					type="password"
					autocomplete="current-password"
					required
					maxlength="128"
				/></label
			>
			{#if !auth.initialPasswordChangeRequired}<label
					>Authenticator or recovery code <span>if enabled</span><input
						bind:value={mfaCode}
						autocomplete="one-time-code"
					/></label
				>{/if}
			{#if error}<p class="error" role="alert">{error}</p>{/if}
			<button
				type="submit"
				disabled={submitting ||
					auth.bootstrapRequired ||
					(auth.initialPasswordChangeRequired && !auth.initialLoginAllowed)}
				>{submitting ? 'Signing in…' : 'Sign in'}</button
			>
			<p class="transport">
				Use the public HTTPS address whenever you are outside your trusted home network.
			</p>
		</form>
	</section>
</main>

<style>
	.login-page {
		min-height: 100dvh;
		display: grid;
		grid-template-columns: minmax(20rem, 1.15fr) minmax(22rem, 0.85fr);
		background: var(--watch-canvas);
		color: var(--text);
	}
	.login-story {
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		overflow: hidden;
		padding: clamp(2rem, 5vw, 5.5rem);
		background: var(--ink);
		position: relative;
	}
	.login-story::after {
		content: '';
		position: absolute;
		z-index: 0;
		width: 28rem;
		height: 28rem;
		right: -9rem;
		bottom: -10rem;
		border-radius: 42% 58% 62% 38%;
		background: var(--coral);
		opacity: 0.95;
		transform: rotate(16deg);
		pointer-events: none;
	}
	.story-copy {
		position: absolute;
		z-index: 1;
		top: 50%;
		right: clamp(2rem, 5vw, 5.5rem);
		left: clamp(2rem, 5vw, 5.5rem);
		transform: translateY(-50%);
	}
	h1 {
		margin: 0.8rem 0 1rem;
		font: 750 clamp(3.6rem, 7vw, 7.5rem)/0.86 var(--font-display);
		letter-spacing: -0.07em;
	}
	h1 em {
		color: var(--sun);
		font-style: normal;
	}
	.story-copy > p {
		max-width: 38rem;
		color: var(--subdued);
		font-size: clamp(1rem, 1.5vw, 1.25rem);
	}
	.login-panel {
		display: grid;
		place-items: center;
		padding: 1.5rem;
		background: var(--paper);
		color: var(--ink);
	}
	form {
		width: min(27rem, 100%);
		display: grid;
		gap: 1rem;
	}
	.form-heading {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 0.8rem;
	}
	.form-heading p,
	.form-heading h2 {
		margin: 0;
	}
	.form-heading p {
		color: #586071;
		font-size: 0.78rem;
		font-weight: 750;
	}
	.form-heading h2 {
		font: 760 2rem/1.05 var(--font-display);
		letter-spacing: -0.035em;
	}
	.lock-tile {
		display: grid;
		width: 3.35rem;
		aspect-ratio: 1;
		place-items: center;
		border-radius: 1rem;
		background: var(--aqua);
	}
	label {
		display: grid;
		gap: 0.42rem;
		font-size: 0.77rem;
		font-weight: 800;
	}
	label span {
		color: #687184;
		font-weight: 600;
	}
	input {
		width: 100%;
		min-height: 3.1rem;
		border: 1px solid #cbd0da;
		border-radius: 0.82rem;
		background: #fff;
		padding: 0.72rem 0.85rem;
		color: var(--ink);
		font: inherit;
	}
	input:focus {
		outline: 3px solid color-mix(in oklch, var(--periwinkle) 30%, transparent);
		border-color: var(--periwinkle);
	}
	button {
		min-height: 3.2rem;
		border: 0;
		border-radius: 0.88rem;
		background: var(--coral);
		color: var(--ink);
		font-weight: 850;
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
	.error,
	.bootstrap {
		margin: 0;
		border-radius: 0.8rem;
		padding: 0.8rem 0.9rem;
		font-size: 0.78rem;
	}
	.error {
		background: #ffe3e3;
		color: #861c28;
	}
	.bootstrap {
		display: grid;
		gap: 0.25rem;
		background: #fff0be;
	}
	.bootstrap code {
		overflow-wrap: anywhere;
	}
	.transport {
		margin: 0.3rem 0 0;
		color: #687184;
		text-align: center;
		font-size: 0.7rem;
	}
	@media (max-width: 760px) {
		.login-page {
			grid-template-columns: 1fr;
		}
		.login-story {
			min-height: 18rem;
			padding: 1.5rem;
		}
		.story-copy > p {
			display: none;
		}
		.story-copy {
			right: 1.5rem;
			left: 1.5rem;
		}
		h1 {
			font-size: clamp(3rem, 16vw, 5rem);
		}
		.login-panel {
			padding-block: 2.2rem;
		}
	}
</style>
