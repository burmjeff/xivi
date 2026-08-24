<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	onMount(() => goto('/studio/settings', { replaceState: true }));
</script>

<p class="sr-only">Opening Studio Settings…</p>
