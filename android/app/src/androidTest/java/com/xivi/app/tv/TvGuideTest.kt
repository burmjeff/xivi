package com.xivi.app.tv

import android.app.Application
import androidx.activity.ComponentActivity
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.media3.common.util.UnstableApi
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.time.Instant
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.ConcurrentHashMap

@UnstableApi
@OptIn(ExperimentalTestApi::class)
class TvGuideTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    @Test fun largeGuidePreservesTimeAndSurfaceWhileBrowsingAndExitsWithoutPip() {
        compose.activity.requestedOrientation = android.content.pm.ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
        compose.waitUntil(10_000) { compose.activity.resources.configuration.orientation == android.content.res.Configuration.ORIENTATION_LANDSCAPE }
        val app = compose.activity.application as Application
        val origin = "https://guide-test-${UUID.randomUUID()}.example"
        val preferenceFile = "guide_test_${UUID.randomUUID()}"
        val base = System.currentTimeMillis().let { it - it % 3_600_000 }
        val mediaRequests = AtomicInteger()
        val releases = AtomicInteger()
        val activeViewers = ConcurrentHashMap.newKeySet<String>()
        val maximumViewers = AtomicInteger()
        fun channel(id: Long, guide: Boolean): JSONObject {
            val items = JSONArray()
            fun programme(pid: Long, title: String, from: Long, to: Long) = JSONObject().put("id", pid).put("title", title)
                .put("start", Instant.ofEpochMilli(from)).put("end", Instant.ofEpochMilli(to)).put("description", "Synthetic TV test programme")
            if (guide && id % 10 != 0L) {
                if (id % 2 == 0L) { items.put(programme(id * 10, "Early show", base, base + 1_800_000)); items.put(programme(id * 10 + 1, "Later show", base + 1_800_000, base + 7_200_000)) }
                else items.put(programme(id * 10, "Long show", base, base + 7_200_000))
            }
            return JSONObject().put("id", id).put("number", id).put("name", "Channel $id").put("group_id", 1).put("group_name", "Test channels")
                .put("stream_url", "/stream/hls/test-$id").put("programmes", items)
        }
        val transport = OkHttpClient.Builder().addInterceptor { chain ->
            val req = chain.request(); val path = req.url.encodedPath
            if (path.startsWith("/stream/hls/")) {
                mediaRequests.incrementAndGet()
                req.url.queryParameter("playback_id")?.let { id ->
                    activeViewers.add(id); maximumViewers.updateAndGet { maxOf(it, activeViewers.size) }
                }
                val asset = if (path.endsWith(".ts")) "segment000.ts" else "playlist.m3u8"
                val bytes = InstrumentationRegistry.getInstrumentation().context.assets.open("playback/$asset").use { it.readBytes() }
                return@addInterceptor Response.Builder().request(req).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                    .body(bytes.toResponseBody((if (asset.endsWith(".ts")) "video/mp2t" else "application/vnd.apple.mpegurl").toMediaType())).build()
            }
            val response = when {
                path == "/healthz" -> JSONObject().put("status", "ok").put("tv_api_version", 1)
                path == "/api/v2/tv/auth/device" -> JSONObject().put("device_code", "test")
                path == "/api/v2/tv/auth/token" -> JSONObject().put("token_type", "DPoP").put("access_token", "xta_test_token").put("refresh_token", "xtr_test_token")
                    .put("access_expires_at", Instant.now().plusSeconds(900)).put("principal", JSONObject().put("user_id", 1))
                path == "/api/v2/watch/lineups" -> JSONObject().put("items", JSONArray().put(JSONObject().put("id", 1).put("name", "Test lineup")))
                path == "/api/v2/watch/preferences" -> ViewerPreferences().json()
                path.endsWith("/channels") -> {
                    val offset = req.url.queryParameter("cursor")?.toLong() ?: 0L
                    JSONObject().put("items", JSONArray((offset + 1..minOf(offset + 250, 5000)).map { channel(it, false) }))
                        .put("next_cursor", if (offset + 250 < 5000) (offset + 250).toString() else JSONObject.NULL)
                }
                path.endsWith("/guide") -> JSONObject().put("items", JSONArray(req.url.queryParameter("channel_ids")!!.split(',').map { channel(it.toLong(), true) }))
                path == "/api/v2/watch/playback/release" -> {
                    releases.incrementAndGet()
                    val buffer = okio.Buffer(); req.body!!.writeTo(buffer)
                    activeViewers.remove(JSONObject(buffer.readUtf8()).getString("playback_id")); JSONObject()
                }
                else -> JSONObject()
            }
            Response.Builder().request(req).protocol(Protocol.HTTP_1_1).code(200).message("OK").body(response.toString().toResponseBody("application/json".toMediaType())).build()
        }.build()
        val auth = TvAuthRepository(app, transport, preferenceFile)
        auth.configure(origin); auth.beginPairing(); auth.poll("test")
        runBlocking(Dispatchers.IO) { TvSettingsStore(app, auth.cacheNamespace()).save(TvSettings(startup = "guide", hints = false)) }
        lateinit var vm: TvViewModel
        compose.runOnUiThread {
            vm = ViewModelProvider(compose.activity, object : ViewModelProvider.Factory {
                @Suppress("UNCHECKED_CAST") override fun <T : ViewModel> create(modelClass: Class<T>): T = TvViewModel(app, auth) as T
            })[TvViewModel::class.java]
        }
        compose.setContent { TvApp(vm) }
        compose.runOnIdle { vm.onStart() }
        compose.waitUntil(30_000) { vm.repository.catalogComplete.value }
        assertEquals(5000, vm.repository.channels.value.size)
        compose.runOnIdle { vm.focus.value = GuideFocus("1:1", base + 2_700_000); vm.loadGuide() }
        compose.waitUntil(15_000) { vm.repository.channels.value.find { it.key == "1:2" }?.programmes?.isNotEmpty() == true }
        val before = vm.focus.value.time
        compose.onNode(hasContentDescription("Channel guide.", substring = true)).performKeyInput { pressKey(Key.DirectionDown) }
        compose.runOnIdle {
            assertEquals("1:2", vm.focus.value.channelKey); assertEquals(before, vm.focus.value.time)
            assertEquals("Later show", vm.selected()?.at(before)?.title)
            assertEquals("Browsing started a provider stream", 0, mediaRequests.get())
            vm.loadGuide()
        }
        compose.waitForIdle()
        assertEquals("1:2", vm.focus.value.channelKey)
        java.io.File(app.filesDir, "tv-guide-test.png").outputStream().use { output ->
            compose.onRoot().captureToImage().asAndroidBitmap().compress(android.graphics.Bitmap.CompressFormat.PNG, 100, output)
        }
        val initialSettings = vm.settings.value
        for ((density, scale) in listOf(GuideDensity.COMPACT to 1f, GuideDensity.LARGE to 1.4f)) {
            compose.runOnIdle { vm.updateSettings(initialSettings.copy(density = density, textScale = scale, highContrast = scale > 1f, reducedMotion = scale > 1f)) }
            compose.waitForIdle()
            val row = compose.onNodeWithTag("tv-row-1:2").fetchSemanticsNode().boundsInRoot
            // A 10-foot guide must allocate actual readable rows, not merely
            // retain a correct invisible focus model beneath an oversized header.
            if (compose.activity.resources.configuration.screenHeightDp >= 500) {
                assertTrue("Guide row became unreadably short: $row", row.height >= 28 * compose.activity.resources.displayMetrics.density)
            }
            java.io.File(app.filesDir, "tv-guide-${density.name.lowercase()}-test.png").outputStream().use { output ->
                compose.onRoot().captureToImage().asAndroidBitmap().compress(android.graphics.Bitmap.CompressFormat.PNG, 100, output)
            }
        }
        compose.runOnIdle { vm.updateSettings(initialSettings) }
        val player = vm.playback.player
        compose.runOnIdle { vm.tune(vm.repository.channels.value.first()); vm.show(TvSurface.GUIDE) }
        compose.waitUntil(20_000) { vm.playback.status.value.firstFrameMs != null }
        compose.runOnIdle {
            assertSame(player, vm.playback.player)
            vm.back()
            assertEquals(TvSurface.LIVE, vm.surface.value)
            vm.show(TvSurface.QUICK); vm.show(TvSurface.SETTINGS)
            assertTrue(vm.back()); assertEquals(TvSurface.LIVE, vm.surface.value)
            assertFalse("Back at the playback root must exit", vm.back())
            val beforeHold = vm.playback.status.value.identity
            val key = android.view.KeyEvent.KEYCODE_DPAD_DOWN
            for (repeat in 0..4) vm.key(android.view.KeyEvent(0, 0, android.view.KeyEvent.ACTION_DOWN, key, repeat))
            assertEquals("Held keys must only browse, not commit streams", beforeHold, vm.playback.status.value.identity)
            assertEquals("1:6", vm.banner.value?.key)
            vm.key(android.view.KeyEvent(android.view.KeyEvent.ACTION_UP, key))
        }
        compose.waitUntil(20_000) { vm.playback.status.value.channel?.key == "1:6" && vm.playback.status.value.firstFrameMs != null }
        compose.runOnIdle {
            val channels = vm.repository.channels.value
            vm.tune(channels[9]); vm.tune(channels[19]); vm.tune(channels[29])
        }
        compose.waitUntil(20_000) { vm.playback.status.value.channel?.key == "1:30" && vm.playback.status.value.firstFrameMs != null }
        compose.runOnIdle {
            assertEquals("Old viewer identities must be released before a committed tune", 1, maximumViewers.get())
            vm.onStop()
            assertNull(vm.playback.status.value.channel)
            assertFalse(compose.activity.isInPictureInPictureMode)
        }
        compose.waitUntil(10_000) { releases.get() > 0 && activeViewers.isEmpty() }
        app.getSharedPreferences(preferenceFile, android.content.Context.MODE_PRIVATE).edit().clear().commit()
    }
}
