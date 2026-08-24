<script lang="ts">
	import { Monitor, Moon, Sun } from '@lucide/svelte';
	import { preferences, setTheme, type ThemePreference } from '$lib/state/preferences.svelte';
	const order: ThemePreference[] = ['system', 'light', 'dark'];
	function cycle() {
		setTheme(order[(order.indexOf(preferences.theme) + 1) % order.length]);
	}
</script>

<button
	class="app-button app-button--quiet app-button--icon"
	onclick={cycle}
	aria-label={`Theme: ${preferences.theme}. Change theme.`}
	title={`Theme: ${preferences.theme}`}
>
	{#if preferences.theme === 'light'}<Sun size={19} />{:else if preferences.theme === 'dark'}<Moon
			size={19}
		/>{:else}<Monitor size={19} />{/if}
</button>
