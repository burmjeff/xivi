<script lang="ts">
	import '@xivi/app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import { loadPreferences } from '$lib/state/preferences.svelte';
	import { auth, loadSession, refreshSession } from '$lib/state/auth.svelte';
	let { children } = $props();
	const queryClient = new QueryClient({
		defaultOptions: { queries: { staleTime: 20_000, retry: 1, refetchOnWindowFocus: false } }
	});
	let dismissedSecurityNotice = $state(false);
	$effect(() => {
		if (auth.principal?.security_notice) dismissedSecurityNotice = false;
	});
	onMount(() => {
		loadPreferences();
		void loadSession();
		const validateActivity = () => {
			if (document.visibilityState === 'visible') void refreshSession();
		};
		const interval = window.setInterval(() => void refreshSession(true), 30_000);
		window.addEventListener('focus', validateActivity);
		document.addEventListener('visibilitychange', validateActivity);
		return () => {
			window.clearInterval(interval);
			window.removeEventListener('focus', validateActivity);
			document.removeEventListener('visibilitychange', validateActivity);
		};
	});
	let cachedUserID: number | null | undefined;
	$effect(() => {
		if (auth.status === 'loading') return;
		const userID = auth.principal?.user_id ?? null;
		if (userID === null || (cachedUserID !== undefined && cachedUserID !== userID)) {
			queryClient.clear();
		}
		cachedUserID = userID;
		const login = page.url.pathname === '/login';
		if (!auth.principal && !login) {
			const next = `${page.url.pathname}${page.url.search}`;
			void goto(`/login?next=${encodeURIComponent(next)}`, { replaceState: true });
			return;
		}
		if (auth.principal?.must_change_password && page.url.pathname !== '/account' && !login) {
			void goto('/account?password-change=required', { replaceState: true });
			return;
		}
		if (auth.principal?.role !== 'admin' && page.url.pathname.startsWith('/studio')) {
			void goto('/', { replaceState: true });
		}
	});
</script>

<QueryClientProvider client={queryClient}>
	{#if auth.status === 'loading'}
		<div class="session-loading" role="status" aria-live="polite">
			<div class="session-pulse"></div>
			<span>Checking your session…</span>
		</div>
	{:else if page.url.pathname === '/login'}
		{@render children?.()}
	{:else if auth.principal?.must_change_password && page.url.pathname === '/account'}
		{@render children?.()}
	{:else if auth.principal && (!page.url.pathname.startsWith('/studio') || auth.principal.role === 'admin')}
		<AppShell>
			{#if auth.principal.security_notice && !dismissedSecurityNotice}
				<div class="security-notice" role="alert">
					<div>
						<strong>Verification attempts detected</strong>
						<span
							>{auth.principal.security_notice.count} rejected MFA
							{auth.principal.security_notice.count === 1 ? 'attempt was' : 'attempts were'} recorded
							before this sign-in{auth.principal.security_notice.last_ip
								? ` from ${auth.principal.security_notice.last_ip}`
								: ''}.</span
						>
					</div>
					<button type="button" onclick={() => (dismissedSecurityNotice = true)}>Dismiss</button>
				</div>
			{/if}
			{@render children?.()}
		</AppShell>
	{/if}
</QueryClientProvider>

<style>
	.session-loading {
		display: grid;
		min-height: 100dvh;
		place-items: center;
		align-content: center;
		gap: 1rem;
		background: var(--watch-canvas);
		color: var(--muted);
		font-weight: 750;
	}
	.session-pulse {
		width: 3rem;
		aspect-ratio: 1;
		border-radius: 1rem;
		background: var(--aqua);
		box-shadow: -0.8rem 0.8rem 0 var(--coral);
		animation: pulse 1.1s ease-in-out infinite alternate;
	}
	.security-notice {
		position: relative;
		z-index: 20;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin: 0.8rem clamp(1rem, 3vw, 2.4rem) 0;
		border: 1px solid color-mix(in oklch, var(--sun) 62%, var(--line));
		border-radius: 0.9rem;
		background: color-mix(in oklch, var(--sun) 16%, var(--surface));
		padding: 0.75rem 0.9rem;
		color: var(--text);
		font-size: 0.78rem;
	}
	.security-notice div {
		display: grid;
		gap: 0.18rem;
	}
	.security-notice span {
		color: var(--muted);
	}
	.security-notice button {
		min-height: 2rem;
		border: 1px solid var(--line);
		border-radius: 0.65rem;
		background: var(--surface-raised);
		color: var(--text);
		padding: 0.3rem 0.65rem;
		font-weight: 750;
	}
	@keyframes pulse {
		to {
			transform: scale(0.9) rotate(-4deg);
			opacity: 0.72;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.session-pulse {
			animation: none;
		}
	}
</style>
