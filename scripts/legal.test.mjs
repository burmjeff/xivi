import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { legalPlugin, npmNotices, sourceInfo } from './legal.mjs';
import { assertSourceCompliance } from '../.github/scripts/android-release-policy.mjs';

const read = (path) => readFileSync(path, 'utf8');
test('binary publication requires explicit source-compliance confirmation', () => {
	assertSourceCompliance('true');
	for (const value of [undefined, '', 'false', '1', true])
		assert.throws(() => assertSourceCompliance(value));
});
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
test('release retains approval gate and publishes exact-source links and legal assets', () => {
	const workflow = read('.github/workflows/android-release.yml');
	assert.match(workflow, /environment:\s+name: android-release/);
	assert.match(
		workflow,
		/VITE_XIVI_SOURCE_URL: https:\/\/github.com\/\$\{\{ github.repository \}\}\/archive\/\$\{\{ github.sha \}\}.tar.gz/
	);
	const signing = read('.github/scripts/sign-android-release.sh');
	assert.match(signing, /assets\/public\/legal\/LICENSE.txt \| cmp -s LICENSE -/);
	assert.match(signing, /sourceArchive:/);
	for (const file of ['LICENSE.txt', 'NOTICE.txt', 'THIRD_PARTY_NOTICES.md', 'LICENSING.md']) {
		assert.ok(signing.includes(file));
		assert.ok(read('.github/scripts/publish-android-release.mjs').includes(`'${file}'`));
	}
});
