import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { appendFileSync, readFileSync } from 'node:fs';
import { checkEnvironment, github, localPlan } from './android-release-policy.mjs';

try {
	const plan = localPlan();
	await checkEnvironment();
	const repo = process.env.GITHUB_REPOSITORY;
	const base = `repos/${repo}`;
	assert.equal(
		await github(`${base}/git/ref/tags/${plan.tag}`, { allowMissing: true }),
		null,
		'The release tag already exists; never replace an existing release'
	);
	assert.equal(
		await github(`${base}/releases/tags/${plan.tag}`, { allowMissing: true }),
		null,
		'The release already exists; inspect any failed draft manually'
	);
	const metadata = JSON.parse(readFileSync('release/release.json', 'utf8'));
	assert.equal(metadata.sourceCommit, process.env.GITHUB_SHA);
	assert.equal(metadata.applicationId, 'com.xivi.app');
	assert.equal(metadata.version, plan.version);
	assert.equal(metadata.versionCode, plan.versionCode);
	const preview = process.env.RELEASE_CHANNEL === 'preview';
	const body = [
		`Xivi Android ${plan.version} (versionCode ${plan.versionCode}) — ${preview ? 'preview for hardware testing' : 'stable'}.`,
		'',
		'One APK includes the phone UI and the native Android TV/Google TV UI. Android 9+; no Android Auto or Fire TV support is declared.',
		'Deploy the matching Xivi server first. TV pairing requires `/healthz` to advertise `tv_api_version: 1`.',
		'Install over the existing release-signed application. Do not uninstall or clear data: that removes device pairing.',
		preview
			? 'Preview: hardware acceptance and performance targets are not implied by a successful CI run.'
			: 'Stable publication was explicitly selected and approved through the android-release environment.',
		'',
		`Source commit: ${process.env.GITHUB_SHA}`,
		`Signing certificate SHA-256: ${metadata.signingCertificateSha256}`,
		`Build: ${metadata.workflowRun}`,
		'',
		'Verify downloads against SHA256SUMS. Xivi.apk is identical to the versioned APK.',
		'Releases in a private repository require GitHub authentication and cannot be used by an anonymous Downloader short code.'
	].join('\n');
	const files = [
		`Xivi-${plan.version}.apk`,
		'Xivi.apk',
		'release.json',
		'signing-certificate.txt',
		'SHA256SUMS'
	];
	// Read and hash all assets before creating anything remotely.
	const assets = files.map((name) => {
		const bytes = readFileSync(`release/${name}`);
		return { name, bytes, digest: `sha256:${createHash('sha256').update(bytes).digest('hex')}` };
	});
	assert(assets[0].bytes.equals(assets[1].bytes), 'Stable and versioned APKs must be identical');
	const draft = await github(`${base}/releases`, {
		method: 'POST',
		body: {
			tag_name: plan.tag,
			target_commitish: process.env.GITHUB_SHA,
			name: `Xivi Android ${plan.version}${preview ? ' Preview' : ''}`,
			body,
			draft: true,
			prerelease: preview
		}
	});
	// Upload everything to a draft first, compatible with immutable releases.
	// Failed uploads leave a private draft for inspection, never a partial latest release.
	const uploadBase = new URL(draft.upload_url.replace(/\{.*$/, ''));
	assert.equal(uploadBase.origin, 'https://uploads.github.com');
	assert.equal(uploadBase.pathname, `/repos/${repo}/releases/${draft.id}/assets`);
	for (const asset of assets) {
		const url = new URL(uploadBase);
		url.searchParams.set('name', asset.name);
		const response = await fetch(url, {
			method: 'POST',
			headers: {
				Authorization: `Bearer ${process.env.GH_TOKEN}`,
				'Content-Type': asset.name.endsWith('.apk')
					? 'application/vnd.android.package-archive'
					: 'application/octet-stream',
				'X-GitHub-Api-Version': '2022-11-28'
			},
			body: asset.bytes,
			redirect: 'error',
			signal: AbortSignal.timeout(180000)
		});
		assert(
			response.ok,
			`Upload failed for ${asset.name} (${response.status}); inspect the draft, do not overwrite a published release`
		);
		const uploaded = await response.json();
		assert.equal(uploaded.size, asset.bytes.length);
		if (uploaded.digest)
			assert.equal(uploaded.digest, asset.digest, 'GitHub asset digest mismatch');
	}
	const published = await github(`${base}/releases/${draft.id}`, {
		method: 'PATCH',
		body: {
			draft: false,
			prerelease: preview,
			make_latest: preview ? 'false' : 'true'
		}
	});
	const result = `Published [${plan.tag}](${published.html_url}). ${preview ? 'Preview; the stable/latest download was not changed.' : 'Stable; Xivi.apk is available through the latest-release asset link.'}\n`;
	console.log(result);
	if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, result);
} catch (error) {
	console.error(error.message);
	process.exitCode = 1;
}
