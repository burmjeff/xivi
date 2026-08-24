<script lang="ts">
	import { ChevronDown } from '@lucide/svelte';
	import type { LineupSummary } from '$lib/api/types';
	import { selectLineup } from '$lib/state/preferences.svelte';
	let { items, value } = $props<{ items: LineupSummary[]; value: number | null }>();
	function change(event: Event) {
		selectLineup(Number((event.currentTarget as HTMLSelectElement).value));
	}
</script>

<label class="lineup-picker"
	><span class="sr-only">Selected lineup</span><select onchange={change} value={value ?? ''}
		>{#each items as lineup}<option value={lineup.id}>{lineup.name}</option>{/each}</select
	><ChevronDown size={17} aria-hidden="true" /></label
>

<style>
	.lineup-picker {
		position: relative;
		display: inline-flex;
		align-items: center;
	}
	select {
		min-height: 2.75rem;
		appearance: none;
		border: 1px solid var(--line);
		border-radius: 0.85rem;
		background: var(--surface);
		padding: 0.55rem 2.5rem 0.55rem 0.85rem;
		color: var(--text);
		font-weight: 750;
		cursor: pointer;
	}
	.lineup-picker :global(svg) {
		position: absolute;
		right: 0.75rem;
		pointer-events: none;
		color: var(--muted);
	}
</style>
