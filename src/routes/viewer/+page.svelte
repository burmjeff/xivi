<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	onMount(() => goto('/', { replaceState: true }));
</script>

<p class="sr-only">Opening Watch…</p>
