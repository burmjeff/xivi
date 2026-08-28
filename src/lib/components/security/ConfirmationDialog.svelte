<script lang="ts">
	import { TriangleAlert, X } from '@lucide/svelte';
	import { Dialog } from 'bits-ui';
	import { XiviAPIError } from '$lib/api/client';

	type Props = {
		open?: boolean;
		title: string;
		description: string;
		confirmLabel?: string;
		cancelLabel?: string;
		tone?: 'danger' | 'warning';
		onconfirm: () => void | Promise<void>;
	};

	let {
		open = $bindable(false),
		title,
		description,
		confirmLabel = 'Continue',
		cancelLabel = 'Cancel',
		tone = 'danger',
		onconfirm
	}: Props = $props();
	let submitting = $state(false);
	let error = $state('');

	$effect(() => {
		if (open) return;
		error = '';
	});

	function errorMessage(caught: unknown) {
		return caught instanceof XiviAPIError
			? caught.detail.message
			: 'The request could not be completed.';
	}

	async function confirm(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = '';
		try {
			await onconfirm();
			open = false;
		} catch (caught) {
			error = errorMessage(caught);
		} finally {
			submitting = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="confirmation-overlay" />
		<Dialog.Content class="confirmation-dialog" aria-describedby="confirmation-description">
			<div class="confirmation-heading">
				<div class:warning={tone === 'warning'} class="confirmation-icon">
					<TriangleAlert size={22} />
				</div>
				<div>
					<Dialog.Title>{title}</Dialog.Title>
					<Dialog.Description id="confirmation-description">{description}</Dialog.Description>
				</div>
				<Dialog.Close
					class="confirmation-close"
					aria-label="Close confirmation"
					disabled={submitting}
				>
					<X size={20} />
				</Dialog.Close>
			</div>
			<form class="confirmation-form" aria-busy={submitting} onsubmit={confirm}>
				{#if error}<p class="confirmation-error" role="alert">{error}</p>{/if}
				<div class="confirmation-actions">
					<Dialog.Close
						class="app-button app-button--secondary"
						type="button"
						disabled={submitting}
					>
						{cancelLabel}
					</Dialog.Close>
					<button
						class:warning-action={tone === 'warning'}
						class="app-button confirmation-action"
						type="submit"
						disabled={submitting}
					>
						{submitting ? 'Working…' : confirmLabel}
					</button>
				</div>
			</form>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	:global(.confirmation-overlay) {
		position: fixed;
		z-index: 78;
		inset: 0;
		background: rgb(8 10 15 / 0.72);
		backdrop-filter: blur(6px);
	}
	:global(.confirmation-dialog) {
		position: fixed;
		z-index: 79;
		top: 50%;
		left: 50%;
		width: min(29rem, calc(100vw - 2rem));
		transform: translate(-50%, -50%);
		border: 1px solid var(--line);
		border-radius: 1.2rem;
		background: var(--surface-raised);
		padding: 1.2rem;
		box-shadow: 0 28px 90px rgb(0 0 0 / 0.45);
	}
	.confirmation-heading {
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: start;
		gap: 0.75rem;
	}
	.confirmation-heading :global(h2),
	.confirmation-heading :global(p) {
		margin: 0;
	}
	.confirmation-heading :global(h2) {
		font: 760 1.35rem/1.1 var(--font-display);
	}
	.confirmation-heading :global(p) {
		margin-top: 0.38rem;
		color: var(--muted);
		font-size: 0.78rem;
		line-height: 1.5;
	}
	.confirmation-icon,
	:global(.confirmation-close) {
		display: grid;
		place-items: center;
	}
	.confirmation-icon {
		width: 2.7rem;
		height: 2.7rem;
		border-radius: 0.8rem;
		background: color-mix(in oklch, var(--error) 22%, var(--surface));
		color: var(--error);
	}
	.confirmation-icon.warning {
		background: color-mix(in oklch, var(--sun) 20%, var(--surface));
		color: color-mix(in oklch, var(--sun) 72%, var(--text));
	}
	:global(.confirmation-close) {
		width: 2.5rem;
		height: 2.5rem;
		border: 0;
		border-radius: 0.7rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	:global(.confirmation-close:hover) {
		background: var(--surface);
		color: var(--text);
	}
	.confirmation-form {
		display: grid;
		gap: 0.8rem;
		margin-top: 1rem;
	}
	.confirmation-error {
		margin: 0;
		border-radius: 0.75rem;
		background: color-mix(in oklch, var(--error) 25%, var(--surface));
		padding: 0.7rem 0.8rem;
		font-size: 0.75rem;
	}
	.confirmation-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.confirmation-action {
		border-color: color-mix(in oklch, var(--error) 75%, transparent);
		background: var(--error);
		color: var(--paper);
	}
	.confirmation-action.warning-action {
		border-color: var(--sun);
		background: var(--sun);
		color: var(--ink);
	}
	@media (max-width: 480px) {
		.confirmation-actions {
			flex-direction: column-reverse;
		}
		.confirmation-actions :global(button) {
			width: 100%;
		}
	}
</style>
