import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
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

test('TV verification preserves per-device reports without ignoring failing tests', () => {
	const workflow = readFileSync('.github/workflows/android-tv.yml', 'utf8');
	assert.match(workflow, /:app:connectedDebugAndroidTest[^\r\n]*--info --stacktrace/);
	assert.match(
		workflow,
		/name: Preserve instrumented test reports, including failures\s+if: always\(\)/
	);
	assert.match(workflow, /uses: actions\/upload-artifact@[0-9a-f]{40}/);
	assert.ok(
		workflow.includes(
			'name: android-tv-tests-api-${{ matrix.api }}-${{ matrix.target }}-${{ matrix.arch }}-${{ github.run_attempt }}'
		)
	);
	assert.ok(workflow.includes('android/app/build/reports/androidTests/connected/'));
	assert.ok(workflow.includes('android/app/build/outputs/androidTest-results/connected/'));
	assert.match(workflow, /retention-days: 7/);
	assert.doesNotMatch(workflow, /continue-on-error: true|\|\| true/);
});

// Execute the actual workflow step with an SDK deliberately absent from PATH.
// These Linux-runner checks also run in preflight, before approval or signing.
for (const sdkExit of [0, 42]) {
	test(
		`signing tools use the installed SDK and JDK 21, preserving sdkmanager exit ${sdkExit}`,
		{ skip: process.platform !== 'linux' },
		(t) => {
			const workflow = readFileSync('.github/workflows/android-release.yml', 'utf8');
			const step = workflow.match(
				/      - name: Prepare Android signing tools without secrets\r?\n        run: \|\r?\n((?:          [^\r\n]*\r?\n)+)/
			);
			assert.ok(step, 'Missing signing-tool preparation step');
			const script = step[1].replace(/^          /gm, '').replace(/\r/g, '');
			const directory = mkdtempSync(join(tmpdir(), 'xivi-signing-tools-'));
			t.after(() => rmSync(directory, { recursive: true, force: true }));
			const sdk = join(directory, 'Android SDK');
			const jdk = join(directory, 'JDK 21');
			const sdkBin = join(sdk, 'cmdline-tools/latest/bin');
			const javaBin = join(jdk, 'bin');
			mkdirSync(sdkBin, { recursive: true });
			mkdirSync(javaBin, { recursive: true });
			writeFileSync(join(javaBin, 'java'), '#!/bin/bash\nprintf "mock JDK 21\\n"\n', {
				mode: 0o755
			});
			writeFileSync(
				join(sdkBin, 'sdkmanager'),
				`#!/bin/bash
set -euo pipefail
[[ "$JAVA_HOME" == "$JAVA_HOME_21_X64" ]]
[[ "$(command -v java)" == "$JAVA_HOME_21_X64/bin/java" ]]
[[ "$(java)" == "mock JDK 21" ]]
[[ "$#" == 2 && "$1" == "--sdk_root=$ANDROID_HOME" && "$2" == 'build-tools;36.0.0' ]]
printf 'sdkmanager invoked\\n'
exit ${sdkExit}
`,
				{ mode: 0o755 }
			);
			const envFile = join(directory, 'github-env');
			const pathFile = join(directory, 'github-path');
			const result = spawnSync(
				'/bin/bash',
				['--noprofile', '--norc', '-eo', 'pipefail', '-c', script],
				{
					encoding: 'utf8',
					env: {
						PATH: '/usr/bin:/bin',
						JAVA_HOME: '/not-jdk-21',
						JAVA_HOME_21_X64: jdk,
						ANDROID_HOME: sdk,
						GITHUB_ENV: envFile,
						GITHUB_PATH: pathFile
					}
				}
			);
			assert.equal(result.status, sdkExit, result.stderr);
			assert.match(result.stdout, /sdkmanager invoked/);
			assert.equal(readFileSync(envFile, 'utf8'), `JAVA_HOME=${jdk}\n`);
			assert.equal(readFileSync(pathFile, 'utf8'), `${javaBin}\n`);
		}
	);
}
