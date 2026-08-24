<script lang="ts">
	let { size = 42, label = true } = $props<{ size?: number; label?: boolean }>();
</script>

<span
	class="brand"
	aria-label={label ? 'Xivi' : undefined}
	aria-hidden={label ? undefined : 'true'}
>
	<img src="/brand/signal-tile.svg" alt="" width={size} height={size} />
	{#if label}<span>Xivi</span>{/if}
</span>

<style>
	.brand {
		display: inline-flex;
		align-items: center;
		gap: 0.7rem;
		font-family: var(--font-display);
		font-size: 1.45rem;
		font-weight: 820;
		letter-spacing: -0.06em;
	}
	img {
		display: block;
		border-radius: calc(var(--radius-sm) * 0.8);
	}
</style>
