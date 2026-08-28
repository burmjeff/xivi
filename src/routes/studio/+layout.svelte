<script lang="ts">
	import { onMount } from 'svelte';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { auth } from '$lib/state/auth.svelte';

	let { children } = $props();
	const queryClient = useQueryClient();

	onMount(() => {
		const events = new EventSource('/api/v2/events');
		let activeJobIds = new Set<number>();
		const refreshResources = (event: Event) => {
			void queryClient.invalidateQueries({ queryKey: ['studio', 'overview'] });
			void queryClient.invalidateQueries({ queryKey: ['studio', 'streams'] });
			try {
				const payload = JSON.parse((event as MessageEvent<string>).data) as {
					jobs?: Array<{ id: number; status: string }>;
				};
				const nextActiveJobIds = new Set(
					(payload.jobs ?? [])
						.filter((job) => job.status === 'queued' || job.status === 'running')
						.map((job) => job.id)
				);
				const finished = [...activeJobIds].some((id) => !nextActiveJobIds.has(id));
				activeJobIds = nextActiveJobIds;
				if (finished) void queryClient.invalidateQueries({ queryKey: ['studio'] });
			} catch {
				// Older servers may emit an untyped snapshot.
			}
		};
		events.addEventListener('snapshot', refreshResources);
		return () => {
			events.removeEventListener('snapshot', refreshResources);
			events.close();
		};
	});
</script>

{#if auth.principal && !auth.principal.mfa_enabled}
	<div class="mfa-warning" role="status">
		<strong>Administrator MFA is off.</strong>
		<span>Protect this internet-facing control plane with a second factor.</span>
		<a href="/account#multi-factor">Enable MFA</a>
	</div>
{/if}
{@render children?.()}

<style>
	.mfa-warning {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		border-bottom: 1px solid color-mix(in oklch, var(--sun) 55%, var(--line));
		background: color-mix(in oklch, var(--sun) 17%, var(--surface));
		padding: 0.7rem clamp(1rem, 3vw, 2.4rem);
		font-size: 0.76rem;
	}
	.mfa-warning span {
		flex: 1;
		color: var(--muted);
	}
	.mfa-warning a {
		color: var(--text);
		font-weight: 850;
		text-decoration: underline;
	}
	@media (max-width: 640px) {
		.mfa-warning {
			align-items: flex-start;
			flex-wrap: wrap;
		}
		.mfa-warning span {
			flex-basis: 100%;
			order: 3;
		}
	}
</style>
