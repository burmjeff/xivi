package com.xivi.app

import android.content.Context
import android.content.pm.ActivityInfo
import android.content.res.Configuration
import android.graphics.Bitmap
import android.os.SystemClock
import android.view.View
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.session.MediaController
import androidx.media3.ui.PlayerView
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.time.Instant
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/** Real bundled web UI -> Capacitor -> service -> decoder, with only HTTP responses replaced. */
@UnstableApi
@RunWith(AndroidJUnit4::class)
class EmbeddedPlaybackFlowTest {
    @Test
    fun channelScreenMiniPlayerPipAndNotificationShareThePlayingVideo() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        val repository = AuthRepository.get(context)
        val store = SecureTokenStore(context)
        val previousServer = store.server()
        val clientField = AuthRepository::class.java.getDeclaredField("authenticatedClient").apply { isAccessible = true }
        val previousClient = clientField.get(repository)
        val segment = instrumentation.context.assets.open("playback/segment000.ts").use { it.readBytes() }
        val playlist = "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:8\n#EXT-X-MEDIA-SEQUENCE:0\n" +
            (1..60).joinToString("") { "#EXT-X-DISCONTINUITY\n#EXTINF:8.0,\nsegment000.ts\n" } + "#EXT-X-ENDLIST\n"
        val client = OkHttpClient.Builder().addInterceptor { chain ->
            val path = chain.request().url.encodedPath
            val (body, mime) = when {
                path.endsWith("segment000.ts") -> segment to "video/mp2t"
                path.startsWith("/stream/hls/") -> playlist.toByteArray() to "application/vnd.apple.mpegurl"
                else -> apiResponse(path).toByteArray() to "application/json"
            }
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .header("Content-Type", mime)
                .body(body.toResponseBody(mime.toMediaType())).build()
        }.build()
        // Test-only transport replacement avoids real credentials or any dependency on a live server.
        clientField.set(repository, client)
        store.setServer("https://playback.test")
        try {
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                try {
                    scenario.onActivity { activity ->
                        activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
                        activity.bridge.webView.loadUrl("https://app.xivi.local/watch/channel/42?lineup=1")
                    }
                    // A fresh emulator can take longer to initialize its first WebView.
                    waitFor(timeoutMs = 60_000) { js(scenario, "!!document.querySelector('.native-video-surface')") == "true" }
                    waitFor { onActivity(scenario) { it.embeddedPlayer!!.playerView.player?.isPlaying == true } }
                    waitFor { onActivity(scenario) { (it.embeddedPlayer!!.playerView.videoSurfaceView as? android.view.TextureView)?.bitmap?.let { bitmap ->
                        val colorful = bitmap.getPixel(bitmap.width / 2, bitmap.height / 2) != android.graphics.Color.BLACK
                        bitmap.recycle()
                        colorful
                    } == true } }
                    assertTrue(js(scenario, "document.querySelector('.now-playing').textContent.includes('Next programme')") == "true")
                    val bounds = JSONObject(js(scenario, "(() => { const r = document.querySelector('.native-video-surface').getBoundingClientRect(); return {x:r.x, y:r.y, width:r.width, height:r.height, viewport:innerWidth}; })()"))
                    scenario.onActivity { activity ->
                        val stage = activity.findViewById<View>(R.id.player_stage)
                        val location = IntArray(2).also { stage.getLocationOnScreen(it) }
                        val webLocation = IntArray(2).also { activity.bridge.webView.getLocationOnScreen(it) }
                        val scale = activity.bridge.webView.width / bounds.getDouble("viewport")
                        assertEquals(webLocation[1] + bounds.getDouble("y") * scale, location[1].toDouble(), 2.0)
                        assertEquals(bounds.getDouble("height") * scale, stage.height.toDouble(), 2.0)
                        activity.embeddedPlayer!!.playerView.hideController()
                    }
                    screenshot("channel-embedded.png")
                    val originalController = onActivity(scenario) { it.embeddedPlayer!!.playerView.player }
                    js(scenario, "document.querySelector('[aria-label=\"Next channel\"]').click()")
                    waitFor { PlaybackCoordinator.state().channelId == 43L && PlaybackCoordinator.state().playing }
                    scenario.onActivity { it.onBackPressedDispatcher.onBackPressed() }
                    waitFor { js(scenario, "!!document.querySelector('.mini-video')") == "true" }
                    waitForMiniPlayer(scenario, portrait = true)
                    assertSame(originalController, onActivity(scenario) { it.embeddedPlayer!!.playerView.player })
                    val initialMini = stageRect(scenario)
                    assertTrue("Mini-player should take less than half the screen width", initialMini.width() < onActivity(scenario) { it.bridge.webView.width } / 2)
                    screenshot("channel-mini.png")
                    gesture(initialMini.exactCenterX(), initialMini.exactCenterY(), 24f, initialMini.top / 2f)
                    waitFor { stageRect(scenario).left < initialMini.left && stageRect(scenario).top < initialMini.top }
                    assertEquals("Dragging must not open the channel", "true", js(scenario, "location.pathname === '/channels'"))
                    assertTrue(PlaybackCoordinator.state().playing)
                    assertSame(originalController, onActivity(scenario) { it.embeddedPlayer!!.playerView.player })
                    screenshot("channel-mini-moved.png")
                    scenario.onActivity { it.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE }
                    waitFor { onActivity(scenario) { it.resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE } }
                    waitForMiniPlayer(scenario, portrait = false)
                    assertTrue(stageRect(scenario).bottom <= onActivity(scenario) { it.findViewById<View>(android.R.id.content).height })
                    scenario.onActivity { it.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT }
                    waitFor { onActivity(scenario) { it.resources.configuration.orientation == Configuration.ORIENTATION_PORTRAIT } }
                    waitForMiniPlayer(scenario, portrait = true)
                    val movedMini = stageRect(scenario)
                    gesture(movedMini.exactCenterX(), movedMini.exactCenterY())
                    waitFor { js(scenario, "location.pathname === '/watch/channel/43'") == "true" }
                    assertSame(originalController, onActivity(scenario) { it.embeddedPlayer!!.playerView.player })
                    scenario.onActivity { it.onBackPressedDispatcher.onBackPressed() }
                    waitFor { js(scenario, "!!document.querySelector('.mini-video')") == "true" }
                    waitFor { stageRect(scenario).left < initialMini.left }
                    // Leaving Xivi from the in-app mini-player enters system PiP.
                    instrumentation.uiAutomation.executeShellCommand("input keyevent KEYCODE_HOME").close()
                    waitFor { onActivity(scenario) { it.isInPictureInPictureMode } }
                    assertTrue(PlaybackCoordinator.state().playing)
                    screenshot("channel-pip.png")
                    context.startActivity(context.packageManager.getLaunchIntentForPackage(context.packageName))
                    waitFor { onActivity(scenario) { !it.isInPictureInPictureMode && it.hasWindowFocus() } }
                    waitFor { js(scenario, "location.pathname === '/watch/channel/43' && !!document.querySelector('.native-video-surface')") == "true" }
                    assertSame(originalController, onActivity(scenario) { it.embeddedPlayer!!.playerView.player })
                    scenario.onActivity { it.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE }
                    waitFor { onActivity(scenario) { it.resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE &&
                        it.findViewById<View>(R.id.player_stage).height == it.findViewById<View>(android.R.id.content).height } }
                    screenshot("channel-landscape.png")
                    scenario.onActivity { it.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT }
                    waitFor { onActivity(scenario) { it.resources.configuration.orientation == Configuration.ORIENTATION_PORTRAIT } }
                    js(scenario, "[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Stop').click()")
                    waitFor { !PlaybackCoordinator.state().active }
                    waitFor { js(scenario, "!![...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Start watching')") == "true" }
                    js(scenario, "[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Start watching').click()")
                    waitFor { PlaybackCoordinator.state().playing && onActivity(scenario) { it.embeddedPlayer!!.playerView.isShown } }
                    scenario.onActivity { it.onBackPressedDispatcher.onBackPressed() }
                    waitFor { js(scenario, "!!document.querySelector('.mini-video')") == "true" }
                    val close = stageRect(scenario, R.id.player_mini_close)
                    gesture(close.exactCenterX(), close.exactCenterY())
                    waitFor { !PlaybackCoordinator.state().active }
                    assertEquals("true", js(scenario, "location.pathname === '/channels'"))
                    js(scenario, "document.querySelector('a[href*=\"/watch/channel/42\"]').click()")
                    waitFor { PlaybackCoordinator.state().channelId == 42L && PlaybackCoordinator.state().playing }
                    instrumentation.uiAutomation.executeShellCommand("input keyevent KEYCODE_HOME").close()
                    waitFor { onActivity(scenario) { it.isInPictureInPictureMode } }
                    assertTrue(PlaybackCoordinator.state().playing)
                    // Allow the Home transition to finish before simulating a tap back into the app.
                    screenshot("channel-auto-pip.png")
                    context.startActivity(PlaybackCoordinator.playerIntent(context))
                    waitFor { onActivity(scenario) { !it.isInPictureInPictureMode && it.hasWindowFocus() } }
                    assertTrue(PlaybackCoordinator.state().playing)
                    scenario.onActivity { assertTrue(it.embeddedPlayer!!.enterPip()) }
                    waitFor { onActivity(scenario) { it.isInPictureInPictureMode } }
                    screenshot("channel-close-pip.png")
                    val rect = android.graphics.Rect()
                    scenario.onActivity {
                        val stage = it.findViewById<View>(R.id.player_stage)
                        val location = IntArray(2).also { point -> stage.getLocationOnScreen(point) }
                        rect.set(location[0], location[1], location[0] + stage.width, location[1] + stage.height)
                    }
                    val now = SystemClock.uptimeMillis()
                    for (action in listOf(android.view.MotionEvent.ACTION_DOWN, android.view.MotionEvent.ACTION_UP)) {
                        val event = android.view.MotionEvent.obtain(now, SystemClock.uptimeMillis(), action,
                            rect.exactCenterX(), rect.exactCenterY(), 0)
                        instrumentation.uiAutomation.injectInputEvent(event, true)
                        event.recycle()
                    }
                    val automation = instrumentation.uiAutomation
                    automation.serviceInfo = automation.serviceInfo.apply {
                        flags = flags or android.accessibilityservice.AccessibilityServiceInfo.FLAG_RETRIEVE_INTERACTIVE_WINDOWS
                    }
                    waitFor {
                        automation.windows.any { window ->
                            findClose(window.root)?.performAction(android.view.accessibility.AccessibilityNodeInfo.ACTION_CLICK) == true
                        }
                    }
                    waitFor { !PlaybackCoordinator.state().active && !PlaybackCoordinator.state().playing }
                } finally {
                    instrumentation.runOnMainSync { if (PlaybackCoordinator.state().active) PlaybackCoordinator.stop(context) }
                }
            }
        } finally {
            clientField.set(repository, previousClient)
            store.setServer(previousServer)
            client.dispatcher.executorService.shutdown()
            client.connectionPool.evictAll()
        }
    }

    private fun waitForMiniPlayer(scenario: ActivityScenario<MainActivity>, portrait: Boolean) {
        // Configuration callbacks precede the WebView resize and its native frame update.
        waitFor { js(scenario, "(innerWidth < innerHeight) === $portrait") == "true" }
        waitFor {
            val css = JSONObject(js(scenario, "(() => { const r = document.querySelector('.mini-video').getBoundingClientRect(); return {width:r.width, viewport:innerWidth}; })()"))
            val expected = css.getDouble("width") * onActivity(scenario) { it.bridge.webView.width } / css.getDouble("viewport")
            onActivity(scenario) { it.findViewById<MiniPlayerLayout>(R.id.player_stage).miniMode } &&
                kotlin.math.abs(stageRect(scenario).width() - expected) <= 2
        }
        InstrumentationRegistry.getInstrumentation().waitForIdleSync()
    }

    private fun stageRect(scenario: ActivityScenario<MainActivity>, viewId: Int = R.id.player_stage): android.graphics.Rect =
        onActivity(scenario) {
            val view = it.findViewById<View>(viewId)
            val location = IntArray(2).also { point -> view.getLocationOnScreen(point) }
            android.graphics.Rect(location[0], location[1], location[0] + view.width, location[1] + view.height)
        }

    private fun gesture(fromX: Float, fromY: Float, toX: Float = fromX, toY: Float = fromY) {
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        val start = SystemClock.uptimeMillis()
        fun send(action: Int, x: Float, y: Float) {
            val event = android.view.MotionEvent.obtain(start, SystemClock.uptimeMillis(), action, x, y, 0)
            assertTrue(automation.injectInputEvent(event, true))
            event.recycle()
        }
        send(android.view.MotionEvent.ACTION_DOWN, fromX, fromY)
        if (fromX != toX || fromY != toY) for (step in 1..12) {
            SystemClock.sleep(20)
            send(android.view.MotionEvent.ACTION_MOVE, fromX + (toX - fromX) * step / 12, fromY + (toY - fromY) * step / 12)
        }
        send(android.view.MotionEvent.ACTION_UP, toX, toY)
    }

    private fun findClose(node: android.view.accessibility.AccessibilityNodeInfo?): android.view.accessibility.AccessibilityNodeInfo? {
        if (node == null) return null
        if (node.contentDescription?.toString() in listOf("Close", "Dismiss") && node.isClickable) return node
        for (index in 0 until node.childCount) findClose(node.getChild(index))?.let { return it }
        return null
    }

    private fun programme(id: Int, title: String, offset: Long) = JSONObject()
        .put("id", id).put("channel_id", "fixture").put("title", title).put("categories", org.json.JSONArray())
        .put("description", "A generated programme for playback testing.")
        .put("start", Instant.now().plusSeconds(offset).toString())
        .put("end", Instant.now().plusSeconds(offset + 3600).toString())

    private fun channel(id: Int) = JSONObject().put("id", id).put("number", id).put("group_id", 1)
        .put("name", if (id == 42) "Test channel with the complete station name" else "Second test channel")
        .put("group_name", "Test lineup").put("stream_url", "/stream/hls/fixture-$id")
        .put("current", programme(1, "Current programme", -1800)).put("next", programme(2, "Next programme", 1800))
        .put("programmes", org.json.JSONArray().put(programme(1, "Current programme", -1800))
            .put(programme(2, "Next programme", 1800)).put(programme(3, "Later programme", 5400)))

    private fun apiResponse(path: String): String = when {
        path == "/api/v2/auth/session" -> """{"user_id":1,"username":"test","display_name":"Test viewer","role":"viewer","must_change_password":false,"mfa_enabled":false,"mfa_required":false,"lineup_ids":[1]}"""
        path == "/api/v2/watch/lineups" -> """{"items":[{"id":1,"name":"Test lineup","channel_count":2}],"total":1}"""
        path.endsWith("/neighbors") -> JSONObject().put("previous", channel(42)).put("next", channel(43)).toString()
        path.matches(Regex("/api/v2/watch/channels/\\d+")) -> channel(path.substringAfterLast('/').toInt()).toString()
        path.endsWith("/groups") -> """{"items":[],"total":0}"""
        path.endsWith("/channels") -> JSONObject().put("items", org.json.JSONArray().put(channel(42)).put(channel(43))).put("total", 2).toString()
        else -> error("Unexpected API request: $path")
    }

    private fun <T> onActivity(scenario: ActivityScenario<MainActivity>, read: (MainActivity) -> T): T {
        val result = AtomicReference<T>()
        scenario.onActivity { result.set(read(it)) }
        return result.get()
    }

    private fun js(scenario: ActivityScenario<MainActivity>, expression: String): String {
        val result = AtomicReference<String>()
        val done = CountDownLatch(1)
        scenario.onActivity { it.bridge.webView.evaluateJavascript(expression) { value -> result.set(value); done.countDown() } }
        assertTrue("WebView did not answer: $expression", done.await(5, TimeUnit.SECONDS))
        return result.get()
    }

    private fun waitFor(timeoutMs: Long = 20_000, predicate: () -> Boolean) {
        val deadline = SystemClock.elapsedRealtime() + timeoutMs
        while (SystemClock.elapsedRealtime() < deadline) {
            if (predicate()) return
            SystemClock.sleep(100)
        }
        fail("The embedded playback flow did not reach the expected state")
    }

    private fun screenshot(name: String) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.waitForIdleSync()
        SystemClock.sleep(1000)
        val bitmap = instrumentation.uiAutomation.takeScreenshot()
        File(instrumentation.targetContext.cacheDir, name).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
        bitmap.recycle()
    }
}
