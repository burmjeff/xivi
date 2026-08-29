import {
	isNativePlatform,
	XiviNative,
	type NativePlaybackItem,
	type NativePlaybackState
} from '$lib/platform/native';

export interface PlaybackController {
	readonly native: boolean;
	playVideo(item: NativePlaybackItem): Promise<void>;
	playAudio(item: NativePlaybackItem): Promise<void>;
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
	playAudio(item: NativePlaybackItem) {
		return XiviNative.playAudio(item);
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
	async playAudio() {}
	async reopen() {}
	async stop() {}
	async state(): Promise<NativePlaybackState> {
		return { active: false, playing: false, audioOnly: false };
	}
	async subscribe() {
		return () => {};
	}
}

export const playbackController: PlaybackController = isNativePlatform()
	? new NativePlaybackController()
	: new BrowserPlaybackController();
