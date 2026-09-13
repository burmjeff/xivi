import {
	isNativePlatform,
	XiviNative,
	type NativePlaybackItem,
	type NativePlaybackState
} from '$lib/platform/native';

export interface PlaybackController {
	readonly native: boolean;
	playVideo(item: NativePlaybackItem): Promise<void>;
	reopen(): Promise<void>;
	stop(): Promise<void>;
	state(): Promise<NativePlaybackState>;
	subscribe(listener: (state: NativePlaybackState) => void): Promise<() => void>;
}

class NativePlaybackController implements PlaybackController {
	readonly native = true;
	playVideo(item: NativePlaybackItem) {
		return XiviNative.playVideo(item);
	}
	reopen() {
		return XiviNative.reopenPlayer();
	}
	stop() {
		return XiviNative.stopPlayback();
	}
	state() {
		return XiviNative.getPlaybackState();
	}
	async subscribe(listener: (state: NativePlaybackState) => void) {
		const handle = await XiviNative.addListener('playbackState', listener);
		return () => void handle.remove();
	}
}

class BrowserPlaybackController implements PlaybackController {
	readonly native = false;
	async playVideo() {}
	async reopen() {}
	async stop() {}
	async state(): Promise<NativePlaybackState> {
		return { active: false, playing: false };
	}
	async subscribe() {
		return () => {};
	}
}

export const playbackController: PlaybackController = isNativePlatform()
	? new NativePlaybackController()
	: new BrowserPlaybackController();
