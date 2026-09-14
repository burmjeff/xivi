<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	let code = $state('');
	let device = $state<{ device_name: string; user_code: string; expires_at: string } | null>(null);
	let busy = $state(false);
	let error = $state('');
	let result = $state('');
	let matches = $state(false);
	onMount(() => {
		code = page.url.searchParams.get('code') ?? '';
		if (code) void preview();
	});
	async function preview() {
		busy = true;
		error = '';
		device = null;
		matches = false;
		try {
			device = await api(`/api/v2/tv/auth/decision?code=${encodeURIComponent(code)}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not look up this pairing code.';
		} finally {
			busy = false;
		}
	}
	async function decide(approve: boolean) {
		if (!device || (approve && !matches)) return;
		busy = true;
		error = '';
		try {
			await api('/api/v2/tv/auth/decision', {
				method: 'POST',
				body: JSON.stringify({ user_code: device.user_code, approve })
			});
			result = approve
				? 'TV approved. Playback will be available on your TV shortly.'
				: 'Pairing rejected. This TV has not been granted access.';
			device = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'The decision could not be saved.';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Pair your TV · Xivi</title></svelte:head>
<section class="pairing">
	<h1>Pair your TV</h1>
	<p>Approve only a TV you are setting up. Scanning a code does not grant access by itself.</p>
	{#if error}<p role="alert">{error}</p>{/if}
	{#if result}<p role="status">{result}</p>
		<a href="/account">Manage paired devices</a>
	{:else}
		<form
			onsubmit={(event) => {
				event.preventDefault();
				void preview();
			}}
		>
			<label for="tv-code">Code shown on your TV</label>
			<input
				id="tv-code"
				bind:value={code}
				maxlength="9"
				autocomplete="off"
				autocapitalize="characters"
				placeholder="ABCD-EFGH"
				required
			/>
			<button class="app-button" disabled={busy}>Look up TV</button>
		</form>
		{#if device}
			<div class="device">
				<h2>{device.device_name}</h2>
				<p class="code">{device.user_code}</p>
				<p>
					This TV gets live viewing and viewer preferences—not Studio or account administration. It
					stays paired until revoked or account security changes.
				</p>
				<label class="match"
					><input type="checkbox" bind:checked={matches} /> The TV name and code match the TV in front
					of me.</label
				>
				<div class="actions">
					<button class="app-button" disabled={busy || !matches} onclick={() => decide(true)}
						>Approve TV</button
					><button
						class="app-button app-button--quiet"
						disabled={busy}
						onclick={() => decide(false)}>Reject</button
					>
				</div>
			</div>
		{/if}
	{/if}
</section>

<style>
	.pairing {
		max-width: 38rem;
		margin: 2rem auto;
		padding: 1.5rem;
	}
	form,
	.device {
		display: grid;
		gap: 1rem;
		margin-top: 1.5rem;
	}
	input:not([type='checkbox']) {
		width: 100%;
		padding: 0.8rem;
		border: 1px solid var(--border);
		border-radius: 0.5rem;
		background: var(--panel);
		color: inherit;
		font-size: 1.2rem;
	}
	.code {
		font-size: 2rem;
		letter-spacing: 0.12em;
		font-weight: 800;
	}
	.match,
	.actions {
		display: flex;
		gap: 0.8rem;
		align-items: center;
	}
	.actions {
		flex-wrap: wrap;
	}
</style>
