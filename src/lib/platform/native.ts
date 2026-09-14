import { Capacitor, registerPlugin, type PluginListenerHandle } from '@capacitor/core';

export interface NativeResponse {
	status: number;
	headers: Record<string, string>;
	body: string;
}

export interface NativePlaybackItem {
	channelId: number;
	lineupId: number;
	name: string;
	programme?: string;
	logoUrl?: string;
	streamUrl: string;
}

export interface NativePlaybackState {
	active: boolean;
	channelId?: number;
	name?: string;
	programme?: string;
	playing: boolean;
	loading?: boolean;
	error?: string;
	lineupId?: number;
}

export interface NativePlayerFrame {
	mode: 'inline' | 'mini' | 'hidden';
	x?: number;
	y?: number;
	width?: number;
	height?: number;
	viewportWidth?: number;
}

interface XiviNativePlugin {
	getServer(): Promise<{ url: string | null }>;
	setServer(options: { url: string }): Promise<{ url: string }>;
	clearServer(): Promise<void>;
	request(options: {
		path: string;
		method: string;
		headers: Record<string, string>;
		body?: string;
	}): Promise<NativeResponse>;
	login(options: { body: string }): Promise<NativeResponse>;
	completeMfa(options: { body: string }): Promise<NativeResponse>;
	refreshSession(): Promise<{ authenticated: boolean }>;
	logout(): Promise<void>;
	cacheArtwork(options: { path: string }): Promise<{ url: string }>;
	playVideo(options: NativePlaybackItem): Promise<void>;
	reopenPlayer(): Promise<void>;
	stopPlayback(): Promise<void>;
	getPlaybackState(): Promise<NativePlaybackState>;
	setPlayerFrame(options: NativePlayerFrame): Promise<void>;
	enterPictureInPicture(): Promise<{ entered: boolean }>;
	addListener(
		eventName: 'openPlayer',
		listener: (item: NativePlaybackItem) => void
	): Promise<PluginListenerHandle>;
	addListener(
		eventName: 'playbackState',
		listener: (state: NativePlaybackState) => void
	): Promise<PluginListenerHandle>;
}

export const XiviNative = registerPlugin<XiviNativePlugin>('XiviNative');

export function isNativePlatform() {
	return Capacitor.isNativePlatform();
}

export async function configuredServer() {
	if (!isNativePlatform()) return null;
	return (await XiviNative.getServer()).url;
}

export async function configureServer(url: string) {
	return (await XiviNative.setServer({ url })).url;
}

export async function resolveArtwork(path?: string) {
	if (!path || !isNativePlatform() || !path.startsWith('/images/')) return path;
	try {
		return (await XiviNative.cacheArtwork({ path })).url;
	} catch {
		return undefined;
	}
}
