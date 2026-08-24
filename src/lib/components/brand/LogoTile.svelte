<script lang="ts">
	let {
		src,
		name,
		size = 'md'
	} = $props<{ src?: string; name: string; size?: 'sm' | 'md' | 'lg' }>();
	let failed = $state(false);
</script>

<span class="logo-tile {size}" title={name}>
	{#if src && !failed}
		<img {src} alt="" onerror={() => (failed = true)} />
	{:else}
		<img src="/brand/signal-tile.svg" alt="" />
	{/if}
</span>

<style>
	.logo-tile {
		display: grid;
		flex: none;
		place-items: center;
		overflow: hidden;
		border: 1px solid var(--line);
		border-radius: 0.9rem;
		background: var(--paper);
	}
	.logo-tile.sm {
		width: 2.3rem;
		height: 2.3rem;
		border-radius: 0.65rem;
	}
	.logo-tile.md {
		width: 3.2rem;
		height: 3.2rem;
	}
	.logo-tile.lg {
		width: 5.2rem;
		height: 5.2rem;
		border-radius: 1.2rem;
	}
	img {
		width: 100%;
		height: 100%;
		object-fit: contain;
	}
</style>
