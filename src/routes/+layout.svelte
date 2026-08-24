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
		let activeJobIds = new Set<number>();
		const refreshResources = (event: Event) => {
			void queryClient.invalidateQueries({ queryKey: ['studio', 'overview'] });
			try {
				const payload = JSON.parse((event as MessageEvent<string>).data) as {
					jobs?: Array<{ id: number; status: string }>;
				};
				const nextActiveJobIds = new Set(
					(payload.jobs ?? [])
						.filter((job) => job.status === 'queued' || job.status === 'running')
						.map((job) => job.id)
				);
				const jobFinished = [...activeJobIds].some((id) => !nextActiveJobIds.has(id));
				activeJobIds = nextActiveJobIds;
				if (jobFinished) void queryClient.invalidateQueries({ queryKey: ['studio'] });
			} catch {
				// The overview still refreshes if an older server sends an untyped event.
			}
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
