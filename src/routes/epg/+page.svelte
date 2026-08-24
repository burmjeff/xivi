<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	onMount(() => goto('/studio/guide-data', { replaceState: true }));
</script>

<p class="sr-only">Opening Guide Data…</p>
