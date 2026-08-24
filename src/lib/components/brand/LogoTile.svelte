<script lang="ts">
	let {
		src,
		name,
		size = 'md',
		contrast = false
	} = $props<{ src?: string; name: string; size?: 'sm' | 'md' | 'lg'; contrast?: boolean }>();
	let failed = $state(false);
</script>

<span class="logo-tile {size}" class:contrast title={name}>
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
	.logo-tile.contrast {
		border-color: color-mix(in oklch, var(--text) 24%, var(--line));
		background-color: oklch(60% 0.025 260);
		background-image:
			linear-gradient(45deg, oklch(68% 0.022 260) 25%, transparent 25%),
			linear-gradient(-45deg, oklch(68% 0.022 260) 25%, transparent 25%),
			linear-gradient(45deg, transparent 75%, oklch(68% 0.022 260) 75%),
			linear-gradient(-45deg, transparent 75%, oklch(68% 0.022 260) 75%);
		background-position:
			0 0,
			0 0.4rem,
			0.4rem -0.4rem,
			-0.4rem 0;
		background-size: 0.8rem 0.8rem;
		box-shadow: inset 0 0 0 1px oklch(100% 0 0 / 0.12);
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
	.logo-tile.contrast img {
		box-sizing: border-box;
		padding: 0.22rem;
		filter: drop-shadow(0 1px 1px oklch(16% 0.02 264 / 0.72))
			drop-shadow(0 0 1px oklch(100% 0 0 / 0.45));
	}
</style>
