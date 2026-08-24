<script lang="ts">
	import SignalMark from '$lib/components/brand/SignalMark.svelte';
	let {
		title,
		message,
		action = 'Open Studio',
		href = '/studio'
	} = $props<{ title: string; message: string; action?: string; href?: string }>();
</script>

<section class="empty-state">
	<SignalMark size={56} label={false} />
	<h2>{title}</h2>
	<p>{message}</p>
	<a class="app-button app-button--primary" {href}>{action}</a>
</section>

<style>
	.empty-state {
		display: grid;
		min-height: 24rem;
		place-items: center;
		align-content: center;
		gap: 0.75rem;
		padding: 2rem;
		text-align: center;
	}
	.empty-state h2 {
		margin: 0.4rem 0 0;
		font-size: 2rem;
	}
	.empty-state p {
		max-width: 32rem;
		margin: 0 0 0.7rem;
		color: var(--muted);
	}
</style>
