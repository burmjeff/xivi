<script lang="ts">
	import { LockKeyhole, X } from '@lucide/svelte';
	import { Dialog } from 'bits-ui';
	import { api, XiviAPIError } from '$lib/api/client';

	type Props = {
		open?: boolean;
		reason: string;
		onconfirmed: () => void | Promise<void>;
	};

	let { open = $bindable(false), reason, onconfirmed }: Props = $props();
	let password = $state('');
	let submitting = $state(false);
	let error = $state('');

	$effect(() => {
		if (open) return;
		password = '';
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
			await api('/api/v2/auth/reauth', {
				method: 'POST',
				body: JSON.stringify({ password })
			});
			await onconfirmed();
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
		<Dialog.Overlay class="reauth-overlay" />
		<Dialog.Content class="reauth-dialog" aria-describedby="reauth-description">
			<div class="reauth-heading">
				<div class="reauth-icon"><LockKeyhole size={21} /></div>
				<div>
					<Dialog.Title>Confirm it’s you</Dialog.Title>
					<Dialog.Description id="reauth-description">
						Enter your password to {reason}.
					</Dialog.Description>
				</div>
				<Dialog.Close class="reauth-close" aria-label="Close confirmation">
					<X size={20} />
				</Dialog.Close>
			</div>
			<form class="reauth-form" aria-busy={submitting} onsubmit={confirm}>
				<label>
					Password
					<input
						type="password"
						bind:value={password}
						autocomplete="current-password"
						disabled={submitting}
						required
					/>
				</label>
				{#if error}<p class="reauth-error" role="alert">{error}</p>{/if}
				<div class="reauth-actions">
					<Dialog.Close
						class="app-button app-button--secondary"
						type="button"
						disabled={submitting}
					>
						Cancel
					</Dialog.Close>
					<button class="app-button app-button--primary" type="submit" disabled={submitting}>
						{submitting ? 'Confirming…' : 'Confirm and continue'}
					</button>
				</div>
			</form>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	:global(.reauth-overlay) {
		position: fixed;
		z-index: 80;
		inset: 0;
		background: rgb(8 10 15 / 0.72);
		backdrop-filter: blur(6px);
	}
	:global(.reauth-dialog) {
		position: fixed;
		z-index: 81;
		top: 50%;
		left: 50%;
		width: min(28rem, calc(100vw - 2rem));
		transform: translate(-50%, -50%);
		border: 1px solid var(--line);
		border-radius: 1.2rem;
		background: var(--surface-raised);
		padding: 1.2rem;
		box-shadow: 0 28px 90px rgb(0 0 0 / 0.45);
	}
	.reauth-heading {
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: start;
		gap: 0.75rem;
	}
	.reauth-heading :global(h2),
	.reauth-heading :global(p) {
		margin: 0;
	}
	.reauth-heading :global(h2) {
		font: 760 1.35rem/1.1 var(--font-display);
	}
	.reauth-heading :global(p) {
		margin-top: 0.3rem;
		color: var(--muted);
		font-size: 0.75rem;
	}
	.reauth-icon,
	:global(.reauth-close) {
		display: grid;
		place-items: center;
	}
	.reauth-icon {
		width: 2.7rem;
		height: 2.7rem;
		border-radius: 0.8rem;
		background: var(--aqua);
		color: var(--ink);
	}
	:global(.reauth-close) {
		width: 2.5rem;
		height: 2.5rem;
		border: 0;
		border-radius: 0.7rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	:global(.reauth-close:hover) {
		background: var(--surface);
		color: var(--text);
	}
	.reauth-form {
		display: grid;
		gap: 0.8rem;
		margin-top: 1rem;
	}
	.reauth-form label {
		display: grid;
		gap: 0.35rem;
		color: var(--muted);
		font-size: 0.72rem;
		font-weight: 800;
	}
	.reauth-form input {
		min-height: 2.8rem;
		width: 100%;
		border: 1px solid var(--line);
		border-radius: 0.75rem;
		background: var(--deep);
		padding: 0.6rem 0.75rem;
		color: var(--text);
	}
	.reauth-error {
		margin: 0;
		border-radius: 0.75rem;
		background: color-mix(in oklch, var(--error) 25%, var(--surface));
		padding: 0.7rem 0.8rem;
		font-size: 0.75rem;
	}
	.reauth-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.2rem;
	}
</style>
