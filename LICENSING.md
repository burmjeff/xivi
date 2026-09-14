# Licensing and distribution

Xivi's original code and documentation are licensed under **AGPL-3.0-only**.
The complete, unmodified license is in [LICENSE](LICENSE); [NOTICE](NOTICE)
defines its scope. Version 3 only is intentional: this release does not grant
an automatic choice of future AGPL versions. Third-party material retains its
own license. This guide is operational guidance, not a replacement for the
license or legal advice.

## Using and modifying Xivi

Personal and commercial use are permitted. AGPL does not prohibit paid hosting,
forks, or competitors, and it does not require changes to be submitted upstream.
If you modify Xivi and users interact with that version over a network, offer
those users its corresponding source at no charge, prominently in the interface.
See [AGPL section 13](https://www.gnu.org/licenses/agpl-3.0.html#section13).

Provide source for the version actually running, not just the upstream main
branch. Set `VITE_XIVI_SOURCE_URL` when building the web/Android app to your
matching source archive or source-access page. Docker builds accept the same
name as a build argument. That URL must be reachable by the users entitled to
the source. Without an override, the interface identifies the upstream project
link as such; that default alone is not a source offer for a modified deployment.

Credentials, databases, provider URLs, user media and signing keys are not part
of the source release. Keep them out of archives. Do not remove required build
scripts or replace source with minified bundles.

## Distributing APKs, binaries and containers

- Preserve Xivi's license and notices, and the licenses/notices of dependencies.
- Provide matching corresponding source with each release, including necessary
  build and installation scripts. An APK or image, an SBOM, or a moving branch
  link is not a substitute for corresponding source.
- The Android release workflow links the exact source commit/archive and ships
  the license and notices beside the APK. Keep those sources available to every
  APK recipient; private repository links work only for recipients with access.
- Workflow dispatch requires `source_compliance_confirmed` to be checked after
  this review. It defaults to false and is checked again after signing approval.
  It records a human compliance check, not an automated legal certification.
- Before binary publication, verify the source and notices for all bundled
  dependencies too. Preserve exact-version source for non-system GPL/LGPL/MPL
  components and any modifications. Package-manager downloads or upstream links
  alone must not be assumed to discharge every redistribution obligation.
- For containers, retain `/usr/share/doc/*/copyright`, `/usr/share/common-licenses`
  and ONNX Runtime's notices. Record the exact image digest and Ubuntu source
  package versions, and arrange compliant access to corresponding package source.
- Verify source downloads without maintainer credentials. Do not publish a
  binary to a wider audience than its corresponding-source access permits.

These are release gates, not a declaration that every possible downstream build
or distribution is compliant. See [AGPL section 6](https://www.gnu.org/licenses/agpl-3.0.html#section6).

## Contributions and earlier versions

Contributions to Xivi's original code are accepted under AGPL-3.0-only unless a
different arrangement is explicitly agreed. Contribute only work you have the
right to license; identify copied/adapted material and retain its notices.
Contributing does not transfer copyright to the maintainer or grant a separate
proprietary licensing right. Third-party code is not relicensed by this notice.

This is a licensing change for this version onward, not a history rewrite.
Earlier `MIT` package metadata and `Apache 2.0` Swagger annotations do not get
retroactively replaced. Any valid permissions already granted for earlier
copies remain in place.

## Compatibility review — 14 September 2026

The review found no identified blocker to applying AGPLv3 to Xivi's original
code. It is a scoped engineering review, not a legal opinion or a complete
redistribution/source-availability audit.

- **Ownership:** the substantive commits inspected use the maintainer's author
  identities. One other named author added placeholder README headings/text in
  2023; none of that addition remains in this README. The other automated author
  is Dependabot. No outside copyright claims were found in current application
  source. Git metadata alone cannot prove ownership or absence of copied code.
- **JavaScript:** all 437 lockfile dependency entries were checked. The metadata
  spans permissive licenses, OFL fonts and MPL-2.0 build tooling. The missing
  `svelte-toolbelt@0.10.6` metadata was resolved by its bundled MIT license.
- **Go:** license files for every dependency explicitly required by `go.mod`
  were inspected. Important exceptions to permissive licenses are
  `go-gst` (LGPL-2.1), `go-glib` (LGPL-3.0), and HashiCorp `errwrap`/
  `go-multierror` (MPL-2.0). No secondary-license-incompatibility headers were
  found in the inspected HashiCorp Go files. Preserve their original terms.
- **Android:** inspected cached AndroidX, Kotlin, OkHttp/Okio and related Maven
  license metadata; primarily Apache-2.0, with BSD components. Parent POMs and
  bundled notices must also be retained; cache inspection is not an exhaustive
  resolved-release dependency audit. Capacitor is MIT.
- **Native media:** inspected the current amd64 image's libvips, GStreamer and
  codec notices. FAAD2, libmpeg2 and x264 permit GPL version 2 **or later**, not
  GPL-2.0-only. GStreamer's LGPL core does not mean every plugin or codec has the
  same license. Recheck added plugins; no proprietary-codec linking exception is
  granted by Xivi. Codec patents and media/provider rights are separate issues.

References: [GNU compatibility guidance](https://www.gnu.org/licenses/gpl-faq.html#AllCompatibility),
[Mozilla MPL guidance](https://www.mozilla.org/en-US/MPL/2.0/FAQ/),
[GStreamer licensing](https://gstreamer.freedesktop.org/documentation/frequently-asked-questions/licensing.html).
