import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
	assertApprovalPolicy,
	assertNewVersion,
	assertSourceCompliance,
	buildVersion,
	fingerprint
} from './android-release-policy.mjs';

const source = (name = '1.1.0', code = 2) => `versionName "${name}"\nversionCode ${code}\n`;
const environment = {
	name: 'android-release',
	protection_rules: [
		{ type: 'required_reviewers', reviewers: [{ type: 'User', reviewer: { id: 1 } }] }
	],
	deployment_branch_policy: { custom_branch_policies: true }
};
const branches = { total_count: 1, branch_policies: [{ name: 'main', type: 'branch' }] };

// These contracts read files excluded from Docker's build context. Keep them
// in the release suite run by both preflight and Android TV verification.
test('binary publication requires explicit source-compliance confirmation', () => {
	assertSourceCompliance('true');
	for (const value of [undefined, '', 'false', '1', true])
		assert.throws(() => assertSourceCompliance(value));
});

test('release retains approval gate and publishes exact-source links and legal assets', () => {
	const read = (path) => readFileSync(path, 'utf8');
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

test('reads explicit APK versions, not npm/server versions', () => {
	assert.deepEqual(buildVersion(source()), {
		version: '1.1.0',
		versionCode: 2,
		tag: 'android-v1.1.0'
	});
	assert.match(
		buildVersion(readFileSync('android/app/build.gradle', 'utf8')).version,
		/^\d+\.\d+\.\d+$/
	);
});
test('AGP built-in Kotlin uses compatible processors and compiler versions', () => {
	const root = readFileSync('android/build.gradle', 'utf8');
	const app = readFileSync('android/app/build.gradle', 'utf8');
	const dependencyVersion = (coordinate) => {
		const match = root.match(new RegExp(`classpath '${coordinate}:([^']+)'`));
		assert.ok(match, `Missing build dependency: ${coordinate}`);
		return match[1];
	};
	assert.equal(
		dependencyVersion('com.android.tools.build:gradle-kotlin'),
		dependencyVersion('com.android.tools.build:gradle')
	);
	assert.equal(
		dependencyVersion('org.jetbrains.kotlin:compose-compiler-gradle-plugin'),
		dependencyVersion('org.jetbrains.kotlin:kotlin-gradle-plugin')
	);
	assert.match(app, /apply plugin: 'com\.android\.legacy-kapt'/);
	assert.doesNotMatch(app, /apply plugin: 'org\.jetbrains\.kotlin\.(android|kapt)'/);
	assert.doesNotMatch(app, /applicationVariants|testVariants|unitTestVariants/);
});
test('rejects unsafe, ambiguous or noncanonical versions', () => {
	for (const value of [
		source('1.01.0'),
		source('$(command)'),
		source('1.0.0', 0),
		source('1.0.0', 2100000001),
		source() + source()
	]) {
		assert.throws(() => buildVersion(value));
	}
});
test('both versionName and versionCode increase; released tags cannot be reused', () => {
	assertNewVersion(buildVersion(source()), [buildVersion(source('1.0.0', 1))]);
	for (const old of [source(), source('1.0.0', 2), source('1.2.0', 1)]) {
		assert.throws(() => assertNewVersion(buildVersion(source()), [buildVersion(old)]));
	}
});
test('approval policy accepts only a reviewer-protected main branch', () => {
	assertApprovalPolicy(environment, branches);
	for (const changed of [
		undefined,
		[],
		[{ type: 'wait_timer' }],
		[{ type: 'required_reviewers', reviewers: [] }]
	]) {
		assert.throws(() =>
			assertApprovalPolicy({ ...environment, protection_rules: changed }, branches)
		);
	}
	assert.throws(() =>
		assertApprovalPolicy({ ...environment, deployment_branch_policy: null }, branches)
	);
	for (const changed of [
		{ total_count: 2, branch_policies: branches.branch_policies },
		{ total_count: 1, branch_policies: [{ name: '*', type: 'branch' }] },
		{ total_count: 1, branch_policies: [{ name: 'main', type: 'tag' }] }
	])
		assert.throws(() => assertApprovalPolicy(environment, changed));
});
test('signing pin is a complete SHA-256, with optional separators', () => {
	assert.equal(fingerprint(Array(32).fill('AB').join(':')), 'ab'.repeat(32));
	assert.throws(() => fingerprint(''));
	assert.throws(() => fingerprint('not-a-certificate'));
});
test('release workflow is manual and signing depends on all verification jobs', () => {
	const workflow = readFileSync('.github/workflows/android-release.yml', 'utf8');
	assert.match(workflow, /workflow_dispatch:/);
	assert.doesNotMatch(workflow, /pull_request_target:|secrets: inherit|continue-on-error: true/);
	assert.match(workflow, /needs: \[preflight, security, native-tv, unsigned-apk\]/);
	assert.match(workflow, /environment:\s+name: android-release/);
	assert.match(workflow, /cancel-in-progress: false/);
	assert.match(workflow, /artifact-ids: \$\{\{ needs\.unsigned-apk\.outputs\.artifact_id \}\}/);
});
