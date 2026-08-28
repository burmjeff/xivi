<script lang="ts">
	import '@xivi/app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import { loadPreferences } from '$lib/state/preferences.svelte';
	import { auth, loadSession } from '$lib/state/auth.svelte';
	let { children } = $props();
	const queryClient = new QueryClient({
		defaultOptions: { queries: { staleTime: 20_000, retry: 1, refetchOnWindowFocus: false } }
	});
	onMount(() => {
		loadPreferences();
		void loadSession();
	});
	$effect(() => {
		if (auth.status === 'loading') return;
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
		<AppShell>{@render children?.()}</AppShell>
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
