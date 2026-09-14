// Run after npm run build (or build:mobile) using Playwright on PATH, as in
// native-player-ui.cjs. No live account or server is required.
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const assert = require('node:assert/strict');
const { createRequire } = require('node:module');
const bin = process.env.PATH.split(path.delimiter).find((entry) =>
	fs.existsSync(path.resolve(entry, '../playwright/package.json'))
);
if (!bin) throw Error('Run with Playwright on PATH');
const { chromium } = createRequire(path.resolve(bin, '../playwright/package.json'))('playwright');
const root = path.resolve('build');
const types = {
	'.html': 'text/html',
	'.js': 'text/javascript',
	'.css': 'text/css',
	'.json': 'application/json',
	'.woff2': 'font/woff2',
	'.svg': 'image/svg+xml'
};
const server = http.createServer((req, res) => {
	const pathname = new URL(req.url, 'http://localhost').pathname;
	if (pathname.startsWith('/api/')) {
		res.writeHead(401, { 'Content-Type': 'application/json' });
		res.end(JSON.stringify({ error: { code: 'authentication_required', message: 'Sign in' } }));
		return;
	}
	let file = path.resolve(root, '.' + pathname);
	if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || fs.statSync(file).isDirectory())
		file = path.join(root, 'index.html');
	res.setHeader('Content-Type', types[path.extname(file)] || 'text/plain');
	fs.createReadStream(file).pipe(res);
});
(async () => {
	await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
	const browser = await chromium.launch({
		headless: true,
		executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
	});
	try {
		fs.mkdirSync('tmp', { recursive: true });
		for (const width of [390, 1280]) {
			const page = await browser.newPage({ viewport: { width, height: 900 } });
			const errors = [];
			page.on('pageerror', (error) => errors.push(error.message));
			await page.goto(`http://127.0.0.1:${server.address().port}/about`);
			await page.getByRole('heading', { name: 'About Xivi', exact: true }).waitFor();
			assert.equal(new URL(page.url()).pathname, '/about');
			assert.equal(
				await page.getByRole('link', { name: /Upstream project source/ }).getAttribute('href'),
				'https://github.com/burmjeff/xivi'
			);
			await page.getByText('GNU Affero General Public License v3.0', { exact: true }).click();
			await page
				.locator('details[open] pre')
				.filter({ hasText: 'END OF TERMS AND CONDITIONS' })
				.waitFor();
			assert.equal(
				await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
				true
			);
			await page.getByText('GNU Affero General Public License v3.0', { exact: true }).click();
			await page.screenshot({ path: `tmp/legal-about-${width}.png`, fullPage: true });
			assert.deepEqual(errors, []);
			await page.close();
		}
		console.log(
			'PASS: anonymous About route, license expansion, source link and responsive layouts'
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
