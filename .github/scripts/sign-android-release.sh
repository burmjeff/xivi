#!/usr/bin/env bash
set -euo pipefail
umask 077

# Run only in the approved signing job. Never run Gradle/npm with these secrets.
: "${RUNNER_TEMP:?}"
: "${ANDROID_HOME:?}"
: "${VERSION:?}"
: "${VERSION_CODE:?}"
: "${EXPECTED_APK_SHA256:?}"
: "${XIVI_KEYSTORE_BASE64:?Configure the existing release keystore as an environment secret}"
: "${XIVI_KEYSTORE_PASSWORD:?}"
: "${XIVI_KEY_ALIAS:?}"
: "${XIVI_KEY_PASSWORD:?}"
: "${XIVI_SIGNING_CERT_SHA256:?Pin the existing release certificate in the environment}"

[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && "$VERSION_CODE" =~ ^[0-9]+$ ]]
[[ "$EXPECTED_APK_SHA256" =~ ^[0-9a-f]{64}$ ]]
input_apk="$RUNNER_TEMP/android-unsigned/Xivi-unsigned.apk"
build_tools="$ANDROID_HOME/build-tools/36.0.0"
[[ -f "$input_apk" ]]
printf '%s  %s\n' "$EXPECTED_APK_SHA256" "$input_apk" | sha256sum --check --status

# Inspect the exact artifact that will be signed, not just Gradle configuration.
badging=$("$build_tools/aapt" dump badging "$input_apk")
[[ "$badging" == *"package: name='com.xivi.app' versionCode='$VERSION_CODE' versionName='$VERSION'"* ]]
[[ "$badging" == *"sdkVersion:'28'"* && "$badging" == *"targetSdkVersion:'36'"* ]]
[[ "$badging" != *"application-debuggable"* ]]
# Ensure the APK includes the license and exact source offer from this checkout.
unzip -p "$input_apk" assets/public/legal/LICENSE.txt | cmp -s LICENSE -
# Template expressions below belong to JavaScript, not the shell.
# shellcheck disable=SC2016
unzip -p "$input_apk" assets/public/legal/source.json | node --input-type=module -e '
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const data = JSON.parse(readFileSync(0, "utf8"));
assert.equal(data.sourceUrl, `https://github.com/${process.env.GITHUB_REPOSITORY}/archive/${process.env.GITHUB_SHA}.tar.gz`);
'
expected_cert=$(node --input-type=module -e 'import { fingerprint } from "./.github/scripts/android-release-policy.mjs"; console.log(fingerprint(process.env.XIVI_SIGNING_CERT_SHA256));')

signing_tmp=$(mktemp -d "$RUNNER_TEMP/xivi-sign.XXXXXX")
trap 'rm -f -- "$signing_tmp/release.jks" "$signing_tmp/aligned.apk"; rmdir -- "$signing_tmp"' EXIT
printf '%s' "$XIVI_KEYSTORE_BASE64" | base64 --decode > "$signing_tmp/release.jks"
unset XIVI_KEYSTORE_BASE64
"$build_tools/zipalign" -P 16 -f 4 "$input_apk" "$signing_tmp/aligned.apk"
mkdir -p release
versioned_apk="release/Xivi-$VERSION.apk"
[[ ! -e "$versioned_apk" ]]
"$build_tools/apksigner" sign --ks "$signing_tmp/release.jks" \
  --ks-key-alias "$XIVI_KEY_ALIAS" --ks-pass env:XIVI_KEYSTORE_PASSWORD \
  --key-pass env:XIVI_KEY_PASSWORD --v4-signing-enabled false \
  --out "$versioned_apk" "$signing_tmp/aligned.apk"
unset XIVI_KEYSTORE_PASSWORD XIVI_KEY_PASSWORD
"$build_tools/apksigner" verify --verbose --print-certs "$versioned_apk" > release/signing-certificate.txt
actual_cert=$(sed -n 's/^Signer #1 certificate SHA-256 digest: //p' release/signing-certificate.txt | tr '[:upper:]' '[:lower:]' | tr -d ':\r')
[[ "$actual_cert" == "$expected_cert" ]] || { echo 'Release certificate does not match the pinned existing signing identity' >&2; exit 1; }
"$build_tools/zipalign" -c -P 16 4 "$versioned_apk"
cp "$versioned_apk" release/Xivi.apk
cp LICENSE release/LICENSE.txt
cp NOTICE release/NOTICE.txt
cp THIRD_PARTY_NOTICES.md release/THIRD_PARTY_NOTICES.md
cp LICENSING.md release/LICENSING.md
export VERIFIED_SIGNING_CERT_SHA256="$actual_cert"
node --input-type=module <<'NODE'
import { writeFileSync } from 'node:fs';
writeFileSync('release/release.json', JSON.stringify({
  applicationId: 'com.xivi.app',
  version: process.env.VERSION,
  versionCode: Number(process.env.VERSION_CODE),
  sourceRepository: process.env.GITHUB_REPOSITORY,
  sourceCommit: process.env.GITHUB_SHA,
  sourceArchive: `https://github.com/${process.env.GITHUB_REPOSITORY}/archive/${process.env.GITHUB_SHA}.tar.gz`,
  license: 'AGPL-3.0-only',
  channel: process.env.RELEASE_CHANNEL,
  minSdk: 28,
  targetSdk: 36,
  tvApiVersion: 1,
  signingCertificateSha256: process.env.VERIFIED_SIGNING_CERT_SHA256,
  unsignedApkSha256: process.env.EXPECTED_APK_SHA256,
  workflowRun: `https://github.com/${process.env.GITHUB_REPOSITORY}/actions/runs/${process.env.GITHUB_RUN_ID}`
}, null, 2) + '\n');
NODE
(
  cd release
  sha256sum "Xivi-$VERSION.apk" Xivi.apk release.json signing-certificate.txt LICENSE.txt NOTICE.txt THIRD_PARTY_NOTICES.md LICENSING.md > SHA256SUMS
)
