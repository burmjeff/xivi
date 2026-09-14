// Run after npm run build:mobile:
// npm exec --yes --package=playwright -- node tests/unit/frontend/native-player-ui.cjs
// Uses the local Edge installation and a simulated Capacitor bridge; no live account is needed.
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const assert = require('node:assert/strict');
const { createRequire } = require('node:module');
const bin = process.env.PATH.split(path.delimiter).find((entry) =>
	fs.existsSync(path.resolve(entry, '../playwright/package.json'))
);
if (!bin)
	throw Error(
		'Run with npm exec --package=playwright -- node tests/unit/frontend/native-player-ui.cjs'
	);
const { chromium } = createRequire(path.resolve(bin, '../playwright/package.json'))('playwright');
const root = path.resolve('build');
const mimes = {
	'.html': 'text/html',
	'.js': 'text/javascript',
	'.css': 'text/css',
	'.json': 'application/json',
	'.svg': 'image/svg+xml',
	'.png': 'image/png',
	'.woff2': 'font/woff2'
};
const server = http.createServer((req, res) => {
	let target = path.resolve(root, '.' + new URL(req.url, 'http://localhost').pathname);
	if (!target.startsWith(root + path.sep)) target = path.join(root, 'index.html');
	if (!fs.existsSync(target) || fs.statSync(target).isDirectory())
		target = path.join(root, 'index.html');
	res.setHeader('Content-Type', mimes[path.extname(target)] || 'application/octet-stream');
	fs.createReadStream(target).pipe(res);
});
(async () => {
	await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
	const browser = await chromium.launch({
		headless: true,
		executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
	});
	try {
		const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
		const pageErrors = [];
		page.on('pageerror', (e) => pageErrors.push(e.message));
		await page.addInitScript(() => {
			window.androidBridge = {};
			window.__playCalls = [];
			window.__reopenCalls = 0;
			window.__frames = [];
			window.__state = { active: false, playing: false };
			const listeners = {};
			const emit = () => (listeners.playbackState || []).forEach((fn) => fn(window.__state));
			window.__openPlayer = (item) => (listeners.openPlayer || []).forEach((fn) => fn(item));
			const programme = (id, title, offset) => ({
				id,
				title,
				channel_id: 'fixture',
				categories: [],
				description: title + ' description.',
				start: new Date(Date.now() + offset * 3600000).toISOString(),
				end: new Date(Date.now() + (offset + 1) * 3600000).toISOString()
			});
			const current = programme(1, 'Current programme', -0.5);
			const next = programme(2, 'Next programme', 0.5);
			const channel = (id) => ({
				id,
				number: id,
				name:
					id === 42
						? 'Test channel with a long full station name that wraps onto multiple lines'
						: 'Second channel',
				group_name: 'Test',
				stream_url: '/stream/hls/fixture-' + id,
				current,
				next,
				programmes: [current, next, programme(3, 'Later programme', 1.5)]
			});
			window.Capacitor = {
				PluginHeaders: [
					{
						name: 'XiviNative',
						methods: [
							...[
								'request',
								'getServer',
								'getPlaybackState',
								'setPlayerFrame',
								'enterPictureInPicture',
								'playVideo',
								'reopenPlayer',
								'stopPlayback',
								'cacheArtwork',
								'removeListener'
							].map((name) => ({ name, rtype: 'promise' })),
							{ name: 'addListener', rtype: 'callback' }
						]
					}
				],
				nativeCallback: async (_, method, options, callback) => {
					if (method === 'addListener') (listeners[options.eventName] ||= []).push(callback);
					return 'listener';
				},
				nativePromise: async (_, method, options) => {
					if (method === 'setPlayerFrame') {
						window.__frames.push(options);
						return;
					}
					if (method === 'enterPictureInPicture') return { entered: true };
					if (method === 'getServer') return { url: 'https://playback.test' };
					if (method === 'getPlaybackState') return window.__state;
					if (method === 'playVideo') {
						window.__playCalls.push(options);
						if (window.__rejectNext) {
							window.__rejectNext = false;
							throw Error('Fixture playback failed');
						}
						window.__state = {
							active: true,
							playing: true,
							channelId: options.channelId,
							lineupId: options.lineupId,
							name: options.name
						};
						emit();
						return;
					}
					if (method === 'reopenPlayer') {
						window.__reopenCalls++;
						return;
					}
					if (method === 'stopPlayback') {
						window.__state = { active: false, playing: false };
						emit();
						return;
					}
					if (method === 'request') {
						const route = options.path.split('?')[0];
						let data;
						if (route === '/api/v2/auth/session')
							data = {
								user_id: 1,
								username: 'test',
								display_name: 'Test viewer',
								role: 'viewer',
								must_change_password: false,
								mfa_enabled: false,
								mfa_required: false,
								lineup_ids: [1]
							};
						else if (route === '/api/v2/watch/lineups')
							data = { items: [{ id: 1, name: 'Test lineup', channel_count: 1 }], total: 1 };
						else if (/^\/api\/v2\/watch\/channels\/\d+$/.test(route))
							data = channel(Number(route.split('/').pop()));
						else if (route.endsWith('/neighbors'))
							data = { previous: channel(42), next: channel(43) };
						else if (route.endsWith('/groups')) data = { items: [], total: 0 };
						else if (route.endsWith('/channels'))
							data = { items: [channel(42), channel(43)], total: 2 };
						else throw Error('Unexpected API request: ' + options.path);
						return {
							status: 200,
							headers: { 'content-type': 'application/json' },
							body: JSON.stringify(data)
						};
					}
					if (method === 'removeListener') return;
					throw Error('Unexpected native method: ' + method);
				}
			};
		});
		await page.goto(`http://127.0.0.1:${server.address().port}/watch/channel/42?lineup=1`);
		await page.waitForFunction(() => window.__playCalls.length === 1);
		assert.equal(await page.locator('video').count(), 0);
		assert.equal(await page.locator('.play-prompt').count(), 0);
		await page.waitForFunction(() => window.__frames.at(-1)?.mode === 'inline');
		await page.getByRole('heading', { name: 'Current programme', exact: true }).waitFor();
		await page.getByRole('heading', { name: 'Next programme', exact: true }).waitFor();
		await page.getByRole('heading', { name: 'Later programme', exact: true }).waitFor();
		assert(
			await page
				.locator('.station h1')
				.evaluate((node) => node.clientHeight > parseFloat(getComputedStyle(node).lineHeight)),
			'The full channel title should wrap'
		);
		const stage = await page.locator('.player-stage').boundingBox();
		assert(
			Math.abs(stage.height - (stage.width * 9) / 16) < 2,
			'Portrait video must remain bounded at 16:9'
		);
		assert.equal(
			await page.getByRole('button', { name: 'Return to video', exact: true }).count(),
			0
		);
		await page.getByRole('button', { name: 'Minimize player', exact: true }).click();
		await page.waitForFunction(() => window.__frames.at(-1)?.mode === 'mini');
		const mini = await page.locator('.native-now-playing').boundingBox();
		const navigation = await page.locator('.watch-bottom').boundingBox();
		assert(
			mini.y + mini.height <= navigation.y,
			'The mini-player must leave app navigation accessible'
		);
		await page.getByRole('button', { name: 'Open', exact: true }).click();
		await page.waitForFunction(() => window.__frames.at(-1)?.mode === 'inline');
		assert.equal(
			await page.evaluate(() => window.__playCalls.length),
			1,
			'Restoring the mini-player must not restart the stream'
		);
		await page.getByRole('link', { name: 'Next channel', exact: true }).click();
		await page.waitForFunction(
			() => window.__playCalls.length === 2 && window.__state.channelId === 43
		);
		await page.getByRole('heading', { name: 'Second channel', exact: true }).waitFor();
		// Android's notification event routes back to the current channel from another app screen.
		await page.evaluate(() => window.dispatchEvent(new Event('xiviMinimizePlayer')));
		await page.waitForFunction(() => window.__frames.at(-1)?.mode === 'mini');
		await page.evaluate(() => window.__openPlayer({ channelId: 43, lineupId: 1 }));
		await page.waitForFunction(() => window.__frames.at(-1)?.mode === 'inline');
		assert.equal(await page.evaluate(() => window.__playCalls.length), 2);
		await page.getByRole('button', { name: 'Stop', exact: true }).click();
		await page.waitForFunction(() => window.__frames.at(-1)?.mode === 'hidden');
		await page.getByRole('button', { name: 'Start watching', exact: true }).click();
		await page.waitForFunction(() => window.__playCalls.length === 3 && window.__state.active);
		await page.getByRole('button', { name: 'Stop', exact: true }).click();
		await page.evaluate(() => (window.__rejectNext = true));
		await page.getByRole('button', { name: 'Start watching', exact: true }).click();
		await page.getByText('Fixture playback failed', { exact: true }).waitFor();
		await page.getByRole('button', { name: 'Start watching', exact: true }).click();
		await page.waitForFunction(() => window.__playCalls.length === 5 && window.__state.active);
		await page.getByRole('link', { name: 'Previous channel', exact: true }).click();
		await page.waitForFunction(
			() => window.__playCalls.length === 6 && window.__state.channelId === 42
		);
		assert.deepEqual(pageErrors, []);
		await page.screenshot({ path: 'tmp/native-player-web.png' });
		console.log(
			'PASS: embedded video bounds, wrapping titles, current/upcoming programmes, channel switching, mini-player, notification return without restarting, and stop/retry'
		);
	} finally {
		await browser.close();
		server.close();
	}
})().catch((error) => {
	console.error(error);
	server.close();
	process.exitCode = 1;
});
