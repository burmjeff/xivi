<script lang="ts" module>
	// A replacement surface takes ownership before the previous component's teardown.
	let currentSurface: HTMLElement | undefined;
</script>

<script lang="ts">
	import { XiviNative, type NativePlayerFrame } from '$lib/platform/native';
	let { mode }: { mode: 'inline' | 'mini' } = $props();

	function surface(node: HTMLElement) {
		currentSurface = node;
		let scheduled = 0;
		let previous = '';
		let navigation: Element | null = null;
		let header: Element | null = null;
		function update() {
			scheduled = 0;
			if (currentSurface !== node) return;
			let movementBounds: NativePlayerFrame['movementBounds'];
			if (mode === 'mini') {
				const nextNavigation = document.querySelector('.watch-bottom');
				if (nextNavigation && navigation !== nextNavigation) {
					if (navigation) observer.unobserve(navigation);
					navigation = nextNavigation;
					observer.observe(navigation);
				}
				const dock = navigation?.getBoundingClientRect();
				const nextHeader = document.querySelector('.watch-header');
				if (nextHeader && header !== nextHeader) {
					if (header) observer.unobserve(header);
					header = nextHeader;
					observer.observe(header);
				}
				const top = header?.getBoundingClientRect();
				const card = node.closest<HTMLElement>('.native-now-playing');
				if (card) card.style.bottom = `${dock?.height ? window.innerHeight - dock.top + 12 : 16}px`;
				movementBounds = {
					left: 12,
					top: top?.height ? Math.max(12, top.bottom + 12) : 12,
					right: window.innerWidth - 12,
					bottom: dock?.height ? dock.top - 12 : window.innerHeight - 12
				};
			}
			const rect = node.getBoundingClientRect();
			const frame: NativePlayerFrame = {
				mode,
				x: rect.x,
				y: rect.y,
				width: rect.width,
				height: rect.height,
				viewportWidth: window.innerWidth,
				...(movementBounds ? { movementBounds } : {})
			};
			const key = JSON.stringify(frame);
			if (key === previous) return;
			previous = key;
			void XiviNative.setPlayerFrame(frame).catch(() => {
				previous = '';
			});
		}
		function schedule() {
			if (!scheduled) scheduled = requestAnimationFrame(update);
		}
		const observer = new ResizeObserver(schedule);
		observer.observe(node);
		window.addEventListener('resize', schedule);
		window.addEventListener('scroll', schedule, true);
		window.visualViewport?.addEventListener('resize', schedule);
		schedule();
		return {
			destroy() {
				cancelAnimationFrame(scheduled);
				observer.disconnect();
				window.removeEventListener('resize', schedule);
				window.removeEventListener('scroll', schedule, true);
				window.visualViewport?.removeEventListener('resize', schedule);
				if (currentSurface === node) {
					currentSurface = undefined;
					void XiviNative.setPlayerFrame({ mode: 'hidden' }).catch(() => {});
				}
			}
		};
	}
</script>

<div
	class="native-video-surface"
	class:mini={mode === 'mini'}
	use:surface
	aria-label="Live video"
></div>

<style>
	.native-video-surface {
		width: 100%;
		height: 100%;
		background: #000;
	}
	.native-video-surface.mini {
		background: transparent;
	}
</style>
