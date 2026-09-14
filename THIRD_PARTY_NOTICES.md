# Third-party software

Xivi's AGPL-3.0-only license applies to its original code, not to separately
licensed dependencies, fonts, icons, generated wrappers or user-provided media.
Keep upstream copyright, license and NOTICE files when redistributing them.

| Component                                                      | Upstream license / source                                                |
| -------------------------------------------------------------- | ------------------------------------------------------------------------ |
| Svelte / SvelteKit, Capacitor, TanStack, Bits UI, Media Chrome | MIT; exact versions in `package-lock.json`                               |
| HLS.js and Atlassian Pragmatic Drag and Drop                   | Apache-2.0                                                               |
| Lucide icons                                                   | ISC; derived Feather material has MIT notices                            |
| Bricolage Grotesque and Manrope fonts                          | SIL Open Font License 1.1; retain font copyright and reserved-name terms |
| Lightning CSS (build tool)                                     | MPL-2.0                                                                  |
| Go bindings `go-gst` / `go-glib`                               | LGPL-2.1 / LGPL-3.0; exact versions in `go.mod`                          |
| HashiCorp `errwrap` and `go-multierror`                        | MPL-2.0                                                                  |
| Other declared Go modules                                      | Predominantly MIT, BSD and Apache-2.0; see each module's license         |
| AndroidX / Media3 / Compose / Room / DataStore                 | Apache-2.0; see resolved Maven artifacts                                 |
| Kotlin, OkHttp / Okio, ZXing                                   | Apache-2.0, with separately licensed bundled material                    |
| Gradle wrapper                                                 | Apache-2.0; original wrapper headers remain intact                       |
| GStreamer / libvips                                            | LGPL; individual codecs/plugins may have different terms                 |
| FAAD2 / libmpeg2 / x264                                        | GPL-2.0-or-later in the inspected Ubuntu image                           |
| ONNX Runtime                                                   | MIT and bundled third-party notices under `/usr/local/onnxruntime`       |

This table is an orientation, not a replacement for the complete dependency
licenses or a complete source offer. npm package contents, Go module archives,
Maven POMs/JARs/AARs and Ubuntu package copyright files are the authoritative
per-version inputs. Build-time web dependency notices are included in the
application's About page. Follow [LICENSING.md](LICENSING.md) before publishing
any binary, especially a container bundling GPL/LGPL codecs.

FAAD2 attribution: Code from FAAD2 is copyright (c) Nero AG, www.nero.com.

Downloaded embedding models retain their own licenses and are not relicensed
as Xivi code. Review the chosen model's upstream terms before redistribution.
Channel artwork, programme data and streams remain subject to their owners'
rights and providers' terms. Xivi grants no rights to that content.
