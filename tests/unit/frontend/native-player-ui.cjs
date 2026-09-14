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
			window.__state = { active: false, playing: false };
			const listeners = [];
			const emit = () => listeners.forEach((fn) => fn(window.__state));
			window.Capacitor = {
				PluginHeaders: [
					{
						name: 'XiviNative',
						methods: [
							...[
								'request',
								'getServer',
								'getPlaybackState',
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
					if (method === 'addListener') listeners.push(callback);
					return 'listener';
				},
				nativePromise: async (_, method, options) => {
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
						else if (route === '/api/v2/watch/channels/42')
							data = {
								id: 42,
								number: 42,
								name: 'Test channel',
								group_name: 'Test',
								stream_url: '/stream/hls/fixture'
							};
						else if (route.endsWith('/neighbors')) data = { previous: null, next: null };
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
		await page.getByRole('button', { name: 'Return to video', exact: true }).click();
		assert.equal(await page.evaluate(() => window.__reopenCalls), 1);
		await page.getByRole('button', { name: 'Stop', exact: true }).click();
		await page.getByRole('button', { name: 'Start watching', exact: true }).click();
		await page.waitForFunction(() => window.__playCalls.length === 2 && window.__state.active);
		await page.getByRole('button', { name: 'Stop', exact: true }).click();
		await page.evaluate(() => (window.__rejectNext = true));
		await page.getByRole('button', { name: 'Start watching', exact: true }).click();
		await page.getByText('Fixture playback failed', { exact: true }).waitFor();
		await page.getByRole('button', { name: 'Start watching', exact: true }).click();
		await page.waitForFunction(() => window.__playCalls.length === 4 && window.__state.active);
		const box = await page
			.getByRole('button', { name: 'Return to video', exact: true })
			.boundingBox();
		const stage = await page.locator('.player-stage').boundingBox();
		assert(
			box.y >= stage.y && box.y + box.height <= stage.y + stage.height,
			'Native controls must fit in the portrait stage'
		);
		assert.deepEqual(pageErrors, []);
		await page.screenshot({ path: 'tmp/native-player-web.png' });
		console.log(
			'PASS: native initial play, return to video, stop/start, failed start/retry, and visible portrait controls'
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
