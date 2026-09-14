import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { appendFileSync, readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function buildVersion(source) {
	const names = [...source.matchAll(/^\s*versionName\s+"(\d+\.\d+\.\d+)"\s*$/gm)];
	const codes = [...source.matchAll(/^\s*versionCode\s+(\d+)\s*$/gm)];
	assert.equal(names.length, 1, 'Expected one literal Android versionName (major.minor.patch)');
	assert.equal(codes.length, 1, 'Expected one literal Android versionCode');
	const version = names[0][1];
	const versionCode = Number(codes[0][1]);
	assert(
		version.split('.').every((part) => String(Number(part)) === part),
		'Noncanonical versionName'
	);
	assert(
		Number.isSafeInteger(versionCode) && versionCode > 0 && versionCode <= 2100000000,
		'Invalid versionCode'
	);
	return { version, versionCode, tag: `android-v${version}` };
}

export function assertNewVersion(current, previous) {
	for (const old of previous) {
		assert.notEqual(
			current.tag,
			old.tag,
			'This Android version is already tagged; never overwrite a release'
		);
		assert(
			current.versionCode > old.versionCode,
			'Increment versionCode beyond every previous Android release, including previews'
		);
		const a = current.version.split('.').map(Number);
		const b = old.version.split('.').map(Number);
		const differing = a.findIndex((value, index) => value !== b[index]);
		assert(
			differing >= 0 && a[differing] > b[differing],
			'versionName must increase beyond previous Android releases'
		);
	}
}

export function assertApprovalPolicy(environment, branches) {
	assert.equal(environment.name, 'android-release', 'Unexpected signing environment');
	assert(
		environment.protection_rules?.some(
			(rule) => rule.type === 'required_reviewers' && rule.reviewers?.length > 0
		),
		'android-release must have at least one required reviewer; an environment name alone is NOT an approval gate'
	);
	assert.equal(
		environment.deployment_branch_policy?.custom_branch_policies,
		true,
		'Restrict android-release to the selected main branch'
	);
	assert.equal(branches.total_count, 1, 'Allow only main to deploy to android-release');
	assert.equal(
		branches.branch_policies?.[0]?.name,
		'main',
		'Allow only main to deploy to android-release'
	);
	// Older branch-only API responses omit type. Explicit tag policies are never allowed.
	assert.equal(
		branches.branch_policies?.[0]?.type ?? 'branch',
		'branch',
		'A tag named main must not grant signing access'
	);
}

export function fingerprint(value) {
	const result = value.trim().replaceAll(':', '').toLowerCase();
	assert(
		/^[0-9a-f]{64}$/.test(result),
		'Configure the SHA-256 fingerprint of the existing RELEASE signing certificate'
	);
	return result;
}

export async function github(path, { method = 'GET', body, allowMissing = false } = {}) {
	assert(process.env.GH_TOKEN, 'Missing GitHub token');
	const response = await fetch(`https://api.github.com/${path}`, {
		method,
		headers: {
			Authorization: `Bearer ${process.env.GH_TOKEN}`,
			Accept: 'application/vnd.github+json',
			'X-GitHub-Api-Version': '2022-11-28',
			...(body ? { 'Content-Type': 'application/json' } : {})
		},
		body: body ? JSON.stringify(body) : undefined,
		signal: AbortSignal.timeout(30000)
	});
	if (response.status === 404 && allowMissing) return null;
	assert(
		response.ok,
		`GitHub ${method} ${path} failed (${response.status}); check repository/environment permissions`
	);
	return response.status === 204 ? null : response.json();
}

export async function checkEnvironment() {
	const repo = process.env.GITHUB_REPOSITORY;
	assert(/^[\w.-]+\/[\w.-]+$/.test(repo ?? ''), 'Invalid repository');
	const base = `repos/${repo}/environments/android-release`;
	assertApprovalPolicy(
		await github(base),
		await github(`${base}/deployment-branch-policies?per_page=100`)
	);
}

export function assertSourceCompliance(value) {
	assert.equal(
		value,
		'true',
		'Review LICENSING.md and confirm corresponding-source and dependency-notice compliance before releasing'
	);
}

export function localPlan() {
	assertSourceCompliance(process.env.XIVI_SOURCE_COMPLIANCE_CONFIRMED);
	assert.equal(
		process.env.GITHUB_REF,
		'refs/heads/main',
		'Run Android release manually from main only'
	);
	assert(['preview', 'stable'].includes(process.env.RELEASE_CHANNEL), 'Choose preview or stable');
	assert.equal(
		execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim(),
		process.env.GITHUB_SHA,
		'Build exactly the workflow dispatch commit'
	);
	const current = buildVersion(readFileSync('android/app/build.gradle', 'utf8'));
	const tags = execFileSync(
		'git',
		['for-each-ref', '--format=%(refname:short)', 'refs/tags/android-v*'],
		{ encoding: 'utf8' }
	).trim();
	const previous = tags
		? tags.split('\n').map((tag) => {
				assert(/^android-v\d+\.\d+\.\d+$/.test(tag), `Unexpected Android tag: ${tag}`);
				const version = buildVersion(
					execFileSync('git', ['show', `${tag}:android/app/build.gradle`], { encoding: 'utf8' })
				);
				assert.equal(version.tag, tag, 'Android tag does not match its source version');
				return version;
			})
		: [];
	assertNewVersion(current, previous);
	return current;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
	try {
		if (process.argv[2] === 'environment') {
			await checkEnvironment();
		} else {
			const plan = localPlan();
			await checkEnvironment();
			if (process.env.GITHUB_OUTPUT)
				appendFileSync(
					process.env.GITHUB_OUTPUT,
					`version=${plan.version}\nversion_code=${plan.versionCode}\ntag=${plan.tag}\n`
				);
			console.log(
				`Validated ${plan.tag} / versionCode ${plan.versionCode}; signing still requires environment approval.`
			);
		}
	} catch (error) {
		console.error(error.message);
		process.exitCode = 1;
	}
}
