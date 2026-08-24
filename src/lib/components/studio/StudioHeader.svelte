<script lang="ts">
	let {
		eyebrow = 'Studio',
		title,
		description,
		children
	} = $props<{
		eyebrow?: string;
		title: string;
		description: string;
		children?: import('svelte').Snippet;
	}>();
</script>

<header class="studio-header">
	<div>
		<p class="eyebrow">{eyebrow}</p>
		<h1>{title}</h1>
		<p>{description}</p>
	</div>
	{#if children}<div class="studio-header-actions">{@render children()}</div>{/if}
</header>

<style>
	.studio-header {
		display: flex;
		align-items: end;
		justify-content: space-between;
		gap: 2rem;
		padding: clamp(1.4rem, 3vw, 2.5rem);
	}
	.studio-header h1 {
		margin: 0;
		font-size: clamp(2.1rem, 4vw, 3.8rem);
		line-height: 1;
	}
	.studio-header p:last-child {
		max-width: 48rem;
		margin: 0.7rem 0 0;
		color: var(--muted);
	}
	.studio-header-actions {
		display: flex;
		gap: 0.55rem;
	}
	@media (max-width: 680px) {
		.studio-header {
			align-items: stretch;
			flex-direction: column;
		}
		.studio-header-actions {
			display: grid;
		}
	}
</style>
