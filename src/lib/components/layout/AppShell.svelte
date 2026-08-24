<script lang="ts">
	import { page } from '$app/state';
	import {
		House,
		ListVideo,
		Radio,
		PanelsTopLeft,
		Rows3,
		Database,
		BookOpen,
		Settings,
		ArrowLeftRight,
		PanelLeftClose,
		PanelLeftOpen
	} from '@lucide/svelte';
	import SignalMark from '$lib/components/brand/SignalMark.svelte';
	import CommandMenu from './CommandMenu.svelte';
	import ThemeSwitcher from '$lib/components/ui/ThemeSwitcher.svelte';
	import { preferences, setStudioNavCollapsed } from '$lib/state/preferences.svelte';
	let { children } = $props();
	let studio = $derived(page.url.pathname.startsWith('/studio'));
	const watchNav = [
		{ href: '/', label: 'Home', icon: House },
		{ href: '/guide', label: 'Guide', icon: ListVideo },
		{ href: '/channels', label: 'Channels', icon: Radio }
	];
	const studioNav = [
		{ href: '/studio', label: 'Overview', icon: PanelsTopLeft },
		{ href: '/studio/lineups', label: 'Lineups', icon: Rows3 },
		{ href: '/studio/sources', label: 'Sources', icon: Database },
		{ href: '/studio/guide-data', label: 'Guide Data', icon: BookOpen },
		{ href: '/studio/settings', label: 'Settings', icon: Settings }
	];
	function active(href: string) {
		return href === '/' || href === '/studio'
			? page.url.pathname === href
			: page.url.pathname.startsWith(href);
	}
</script>

<div class:studio-mode={studio} class:watch-mode={!studio} class="shell">
	{#if studio}
		<aside class:collapsed={preferences.studioNavCollapsed} class="studio-rail">
			<div class="rail-head">
				<a class="rail-brand" href="/studio" aria-label="Xivi Studio"><SignalMark /></a>
				<button
					class="rail-toggle"
					type="button"
					aria-controls="studio-navigation"
					aria-expanded={!preferences.studioNavCollapsed}
					aria-label={preferences.studioNavCollapsed
						? 'Expand Studio navigation'
						: 'Collapse Studio navigation'}
					title={preferences.studioNavCollapsed ? 'Expand navigation' : 'Collapse navigation'}
					onclick={() => setStudioNavCollapsed(!preferences.studioNavCollapsed)}
				>
					{#if preferences.studioNavCollapsed}<PanelLeftOpen size={19} />{:else}<PanelLeftClose
							size={19}
						/>{/if}
				</button>
			</div>
			<nav id="studio-navigation" aria-label="Studio">
				<span class="nav-label">Studio</span>{#each studioNav as item}{@const Icon = item.icon}<a
						href={item.href}
						class:active={active(item.href)}
						aria-label={preferences.studioNavCollapsed ? item.label : undefined}
						title={preferences.studioNavCollapsed ? item.label : undefined}
						><Icon size={19} /><span>{item.label}</span></a
					>{/each}
			</nav>
			<div class="rail-bottom">
				<div class="rail-command">
					<CommandMenu compact iconOnly={preferences.studioNavCollapsed} />
				</div>
				<a
					href="/"
					aria-label={preferences.studioNavCollapsed ? 'Watch' : undefined}
					title={preferences.studioNavCollapsed ? 'Watch' : undefined}
					><ArrowLeftRight size={18} /><span>Watch</span></a
				><ThemeSwitcher />
			</div>
		</aside>
	{:else}
		<header class="watch-header">
			<a href="/" aria-label="Xivi home"><SignalMark /></a>
			<nav aria-label="Watch">
				{#each watchNav as item}<a href={item.href} class:active={active(item.href)}>{item.label}</a
					>{/each}
			</nav>
			<div class="header-actions">
				<CommandMenu compact iconOnly triggerLabel="Search Xivi" />
				<a class="studio-link" href="/studio">Studio</a>
				<ThemeSwitcher />
			</div>
		</header>
	{/if}

	<main
		class:studio-main={studio}
		class:studio-nav-collapsed={studio && preferences.studioNavCollapsed}
		class:watch-main={!studio}
	>
		{@render children?.()}
	</main>

	{#if !studio}<nav class="watch-bottom" aria-label="Watch">
			{#each watchNav as item}{@const Icon = item.icon}<a
					href={item.href}
					class:active={active(item.href)}><Icon size={21} /><span>{item.label}</span></a
				>{/each}<a href="/studio"><PanelsTopLeft size={21} /><span>Studio</span></a>
		</nav>{/if}
</div>

<style>
	.shell {
		min-height: 100dvh;
		background: var(--deep);
		color: var(--text);
	}
	.watch-header {
		position: sticky;
		z-index: 50;
		top: 0;
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		height: 4.6rem;
		align-items: center;
		border-bottom: 1px solid var(--line);
		background: color-mix(in oklch, var(--deep) 94%, transparent);
		padding: 0 clamp(1rem, 3vw, 3rem);
		backdrop-filter: blur(18px);
	}
	.watch-header nav {
		display: flex;
		gap: 0.2rem;
		padding: 0.3rem;
		border-radius: 0.85rem;
		background: var(--surface);
	}
	.watch-header nav a {
		border-radius: 0.65rem;
		padding: 0.48rem 0.85rem;
		color: var(--muted);
		font-size: 0.85rem;
		font-weight: 750;
	}
	.watch-header nav a.active {
		background: var(--paper);
		color: var(--ink);
	}
	.header-actions {
		display: flex;
		justify-content: flex-end;
		align-items: center;
		gap: 0.45rem;
	}
	.studio-link {
		min-height: 2.75rem;
		display: inline-flex;
		align-items: center;
		border: 1px solid var(--line);
		border-radius: 0.85rem;
		padding: 0.55rem 0.85rem;
		font-weight: 750;
	}
	.watch-main {
		min-height: calc(100dvh - 4.6rem);
		overflow: hidden;
	}
	.watch-bottom {
		display: none;
	}
	.studio-rail {
		position: fixed;
		z-index: 50;
		inset: 0 auto 0 0;
		display: flex;
		width: 15.5rem;
		flex-direction: column;
		border-right: 1px solid var(--line);
		background: var(--surface);
		padding: 1.1rem 0.8rem;
		transition: width var(--layout) var(--ease-out);
	}
	.rail-head {
		display: flex;
		min-height: 4.4rem;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.4rem;
	}
	.rail-brand {
		min-width: 0;
		padding: 0.25rem 0.6rem;
	}
	.rail-toggle {
		display: grid;
		width: 2.75rem;
		height: 2.75rem;
		flex: none;
		place-items: center;
		border: 1px solid transparent;
		border-radius: 0.75rem;
		background: transparent;
		color: var(--muted);
		cursor: pointer;
	}
	.rail-toggle:hover {
		border-color: var(--line);
		background: var(--surface-raised);
		color: var(--text);
	}
	.studio-rail nav {
		display: grid;
		gap: 0.25rem;
	}
	.nav-label {
		padding: 0.4rem 0.7rem;
		color: var(--muted);
		font-size: 0.7rem;
		font-weight: 850;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.studio-rail nav a,
	.rail-bottom > a {
		display: flex;
		min-height: 2.8rem;
		align-items: center;
		gap: 0.75rem;
		border-radius: 0.75rem;
		padding: 0.65rem 0.75rem;
		color: var(--muted);
		font-weight: 720;
	}
	.studio-rail nav a:hover,
	.rail-bottom > a:hover {
		background: var(--surface-raised);
		color: var(--text);
	}
	.studio-rail nav a.active {
		background: var(--periwinkle);
		color: #fff;
	}
	.rail-bottom {
		margin-top: auto;
		display: grid;
		grid-template-columns: 1fr auto;
		align-items: center;
		gap: 0.35rem;
		border-top: 1px solid var(--line);
		padding-top: 0.7rem;
	}
	.rail-command {
		grid-column: 1/3;
	}
	.studio-main {
		min-height: 100dvh;
		margin-left: 15.5rem;
		background: var(--deep);
		transition: margin-left var(--layout) var(--ease-out);
	}
	.studio-rail.collapsed {
		width: 5rem;
		padding-inline: 0.65rem;
	}
	.studio-rail.collapsed .rail-head {
		min-height: 6.8rem;
		align-items: center;
		flex-direction: column;
		justify-content: flex-start;
	}
	.studio-rail.collapsed .rail-brand {
		padding: 0;
	}
	.studio-rail.collapsed .rail-brand :global(.brand > span),
	.studio-rail.collapsed .nav-label,
	.studio-rail.collapsed nav a span,
	.studio-rail.collapsed .rail-bottom > a span {
		display: none;
	}
	.studio-rail.collapsed nav a,
	.studio-rail.collapsed .rail-bottom > a {
		width: 2.8rem;
		justify-content: center;
		gap: 0;
		margin-inline: auto;
		padding-inline: 0;
	}
	.studio-rail.collapsed .rail-bottom {
		grid-template-columns: 1fr;
		justify-items: center;
	}
	.studio-rail.collapsed .rail-command {
		grid-column: 1;
	}
	.studio-rail.collapsed .rail-bottom > :global(button) {
		justify-self: center;
	}
	.studio-main.studio-nav-collapsed {
		margin-left: 5rem;
	}
	@media (max-width: 900px) {
		.watch-header {
			grid-template-columns: auto 1fr;
		}
		.watch-header nav {
			display: none;
		}
		.studio-link {
			display: none;
		}
		.studio-rail,
		.studio-rail.collapsed {
			position: sticky;
			top: 0;
			width: 100%;
			height: 4.25rem;
			flex-direction: row;
			align-items: center;
			border-right: 0;
			border-bottom: 1px solid var(--line);
			padding: 0.55rem 1rem;
		}
		.rail-brand {
			padding: 0;
		}
		.rail-head,
		.studio-rail.collapsed .rail-head {
			min-height: 0;
			align-items: center;
			flex-direction: row;
		}
		.rail-toggle {
			display: none;
		}
		.studio-rail.collapsed .rail-brand :global(.brand > span) {
			display: inline;
		}
		.studio-rail nav {
			display: flex;
			flex: 1;
			justify-content: center;
			overflow-x: auto;
		}
		.studio-rail nav a {
			min-width: 2.75rem;
			justify-content: center;
			padding: 0.6rem;
		}
		.studio-rail nav a span,
		.nav-label,
		.rail-bottom > a span {
			display: none;
		}
		.rail-bottom,
		.studio-rail.collapsed .rail-bottom {
			display: flex;
			margin: 0;
			border: 0;
			padding: 0;
		}
		.rail-command {
			width: 2.75rem;
		}
		.studio-main,
		.studio-main.studio-nav-collapsed {
			min-height: calc(100dvh - 4.25rem);
			margin-left: 0;
		}
	}
	@media (max-width: 650px) {
		.studio-rail .rail-brand :global(.brand > span),
		.studio-rail.collapsed .rail-brand :global(.brand > span) {
			display: none;
		}
		.watch-header {
			height: 4.15rem;
			padding: 0 0.9rem;
		}
		.watch-main {
			min-height: calc(100dvh - 8.5rem);
			padding-bottom: 4.35rem;
		}
		.watch-bottom {
			position: fixed;
			z-index: 60;
			inset: auto 0.65rem 0.65rem;
			display: grid;
			grid-template-columns: repeat(4, 1fr);
			border: 1px solid var(--line);
			border-radius: 1.15rem;
			background: color-mix(in oklch, var(--surface) 94%, transparent);
			padding: 0.38rem;
			box-shadow: 0 15px 40px rgb(0 0 0 / 0.35);
			backdrop-filter: blur(16px);
		}
		.watch-bottom a {
			display: grid;
			min-height: 3.45rem;
			place-items: center;
			align-content: center;
			gap: 0.2rem;
			border-radius: 0.85rem;
			color: var(--muted);
			font-size: 0.65rem;
			font-weight: 750;
		}
		.watch-bottom a.active {
			background: var(--coral);
			color: var(--ink);
		}
	}
</style>
