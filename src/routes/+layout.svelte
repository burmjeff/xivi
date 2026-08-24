<script lang="ts">
	import '@xivi/app.css';
	import { onMount } from 'svelte';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import { loadPreferences } from '$lib/state/preferences.svelte';
	let { children } = $props();
	const queryClient = new QueryClient({
		defaultOptions: { queries: { staleTime: 20_000, retry: 1, refetchOnWindowFocus: false } }
	});
	onMount(() => {
		loadPreferences();
		const events = new EventSource('/api/v2/events');
		const refreshResources = () => {
			void queryClient.invalidateQueries({ queryKey: ['studio'] });
		};
		events.addEventListener('snapshot', refreshResources);
		return () => {
			events.removeEventListener('snapshot', refreshResources);
			events.close();
		};
	});
</script>

<QueryClientProvider client={queryClient}
	><AppShell>{@render children?.()}</AppShell></QueryClientProvider
>
