import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { legalPlugin, npmNotices, sourceInfo } from './legal.mjs';

// This suite runs in Docker, whose context deliberately excludes .github.
// Release-workflow contracts belong in .github/scripts/android-release.test.mjs.
const read = (path) => readFileSync(path, 'utf8');
test('only Xivi root metadata changes license; bundled dependencies retain their licenses', () => {
	assert.equal(JSON.parse(read('package.json')).license, 'AGPL-3.0-only');
	const packages = JSON.parse(read('package-lock.json')).packages;
	assert.equal(packages[''].license, 'AGPL-3.0-only');
	assert.equal(packages['node_modules/@capacitor/core'].license, 'MIT');
	assert.equal(packages['node_modules/@fontsource-variable/manrope'].license, 'OFL-1.1');
	assert.match(read('LICENSE'), /GNU AFFERO GENERAL PUBLIC LICENSE/);
	assert.match(read('LICENSE'), /END OF TERMS AND CONDITIONS/);
	assert.match(read('NOTICE'), /AGPL-3.0-only/);
	assert.match(read('android/gradlew'), /SPDX-License-Identifier: Apache-2.0/);
});
test('custom source URLs are HTTPS and cannot contain credentials', () => {
	assert.equal(sourceInfo().sourceLabel, 'Upstream project source');
	assert.equal(
		sourceInfo('https://example.org/source.tar.gz').sourceLabel,
		'Source for this build'
	);
	for (const value of [
		'javascript:alert(1)',
		'http://example.org/source',
		'https://user:secret@example.org/',
		'/source'
	]) {
		assert.throws(() => sourceInfo(value));
	}
});
test('bundled notices preserve original copyright text, including missing npm license metadata', () => {
	const text = npmNotices(process.cwd());
	assert.match(text, /svelte-toolbelt@0.10.6/);
	assert.match(text, /Hunter Johnston/);
	assert.match(text, /SIL OPEN FONT LICENSE/);
	assert.match(text, /Cole Bemis/);
});
test('web and native TV receive the same complete bundled license and source offer', () => {
	const url = 'https://example.org/xivi-source.tar.gz';
	const plugin = legalPlugin(process.cwd(), url);
	const assets = [];
	plugin.generateBundle.call({ emitFile: (asset) => assets.push(asset) });
	assert.equal(
		assets.find((asset) => asset.fileName === 'legal/LICENSE.txt').source,
		read('LICENSE')
	);
	assert.equal(
		JSON.parse(assets.find((asset) => asset.fileName === 'legal/source.json').source).sourceUrl,
		url
	);
	assert.match(plugin.load(plugin.resolveId('virtual:xivi-legal')), /Source for this build/);
	assert.match(
		read('android/app/src/main/java/com/xivi/app/tv/TvLegal.kt'),
		/public\/legal\/source.json/
	);
});
