import { browser } from '$app/environment';
export type ThemePreference = 'system' | 'light' | 'dark';
export const preferences = $state({
	theme: 'system' as ThemePreference,
	lineupId: null as number | null,
	channelView: 'grid' as 'grid' | 'list',
	studioNavCollapsed: false
});

export function loadPreferences() {
	if (!browser) return;
	const theme = localStorage.getItem('xivi:theme') as ThemePreference | null;
	const lineupId = Number(localStorage.getItem('xivi:lineup'));
	const channelView = localStorage.getItem('xivi:channel-view');
	const studioNav = localStorage.getItem('xivi:studio-nav');
	if (theme && ['system', 'light', 'dark'].includes(theme)) preferences.theme = theme;
	if (lineupId > 0) preferences.lineupId = lineupId;
	if (channelView === 'grid' || channelView === 'list') preferences.channelView = channelView;
	preferences.studioNavCollapsed = studioNav === 'collapsed';
	applyTheme();
}

export function applyTheme() {
	if (browser) document.documentElement.dataset.theme = preferences.theme;
}
export function setTheme(theme: ThemePreference) {
	preferences.theme = theme;
	if (browser) localStorage.setItem('xivi:theme', theme);
	applyTheme();
}
export function selectLineup(id: number) {
	preferences.lineupId = id;
	if (browser) localStorage.setItem('xivi:lineup', String(id));
}
export function setChannelView(view: 'grid' | 'list') {
	preferences.channelView = view;
	if (browser) localStorage.setItem('xivi:channel-view', view);
}
export function setStudioNavCollapsed(collapsed: boolean) {
	preferences.studioNavCollapsed = collapsed;
	if (browser) localStorage.setItem('xivi:studio-nav', collapsed ? 'collapsed' : 'expanded');
}
