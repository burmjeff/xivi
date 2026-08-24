<script lang="ts">
	import { Play, Clock3 } from '@lucide/svelte';
	import LogoTile from '$lib/components/brand/LogoTile.svelte';
	import type { GuideChannel } from '$lib/api/types';
	let { channel, compact = false } = $props<{ channel: GuideChannel; compact?: boolean }>();
	let progress = $derived.by(() => {
		if (!channel.current) return 0;
		const start = new Date(channel.current.start).getTime(),
			end = new Date(channel.current.end).getTime();
		return Math.max(0, Math.min(100, ((Date.now() - start) / (end - start)) * 100));
	});
	function formatTime(value?: string) {
		return value
			? new Intl.DateTimeFormat([], { hour: 'numeric', minute: '2-digit' }).format(new Date(value))
			: '';
	}
</script>

<article class:compact class="channel-card">
	<div class="card-top">
		<LogoTile src={channel.logo_url} name={channel.name} size={compact ? 'sm' : 'md'} />
		<div class="identity">
			<span class="channel-number tabular">{channel.number}</span>
			<h3>{channel.name}</h3>
		</div>
		<span class="live-pill">Live</span>
	</div>
	<div class="programme">
		<strong>{channel.current?.title ?? 'Schedule unavailable'}</strong>{#if channel.current}<span
				><Clock3 size={13} />{formatTime(channel.current.start)}–{formatTime(
					channel.current.end
				)}</span
			>{:else}<span>Playback is still available</span>{/if}
	</div>
	<div
		class="progress"
		aria-label={channel.current
			? `${Math.round(progress)} percent complete`
			: 'Schedule unavailable'}
	>
		<i style={`width:${progress}%`}></i>
	</div>
	<div class="card-actions">
		{#if channel.next}<p>Next <strong>{channel.next.title}</strong></p>{:else}<p>
				No upcoming schedule
			</p>{/if}<a
			class="play-button"
			href={`/watch/channel/${channel.id}`}
			aria-label={`Play ${channel.name}`}><Play size={18} fill="currentColor" />Play</a
		>
	</div>
</article>

<style>
	.channel-card {
		display: flex;
		min-width: 19.5rem;
		max-width: 23rem;
		min-height: 16rem;
		flex-direction: column;
		scroll-snap-align: start;
		border: 1px solid var(--line);
		border-radius: 1.25rem;
		background: var(--surface);
		padding: 1rem;
		transition:
			transform var(--layout) var(--ease-out),
			border-color var(--micro);
	}
	.channel-card:hover {
		transform: translateY(-4px);
		border-color: color-mix(in oklch, var(--aqua) 50%, var(--line));
	}
	.channel-card.compact {
		min-width: 17rem;
		min-height: 12.5rem;
	}
	.card-top {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.identity {
		min-width: 0;
		flex: 1;
	}
	.channel-number {
		color: var(--muted);
		font-size: 0.72rem;
		font-weight: 750;
	}
	h3 {
		overflow: hidden;
		margin: 0.05rem 0 0;
		font-size: 1rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.programme {
		display: grid;
		gap: 0.35rem;
		margin-top: 1.35rem;
	}
	.programme strong {
		min-height: 2.7rem;
		font-family: var(--font-display);
		font-size: 1.2rem;
		line-height: 1.1;
	}
	.programme span {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		color: var(--muted);
		font-size: 0.75rem;
	}
	.progress {
		height: 0.34rem;
		overflow: hidden;
		border-radius: 99px;
		background: var(--surface-raised);
		margin: 1rem 0;
	}
	.progress i {
		display: block;
		height: 100%;
		border-radius: inherit;
		background: var(--aqua);
	}
	.card-actions {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		margin-top: auto;
	}
	.card-actions p {
		min-width: 0;
		flex: 1;
		margin: 0;
		overflow: hidden;
		color: var(--muted);
		font-size: 0.72rem;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.card-actions p strong {
		color: var(--text);
	}
	.play-button {
		display: inline-flex;
		min-height: 2.75rem;
		align-items: center;
		gap: 0.4rem;
		border-radius: 0.8rem;
		background: var(--coral);
		padding: 0.55rem 0.8rem;
		color: var(--ink);
		font-weight: 820;
	}
</style>
