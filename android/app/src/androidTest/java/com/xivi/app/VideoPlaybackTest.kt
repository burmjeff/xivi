package com.xivi.app

import android.content.pm.ActivityInfo
import android.content.Context
import org.json.JSONObject
import android.content.res.Configuration
import android.graphics.Bitmap
import android.os.SystemClock
import android.view.View
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
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
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

@UnstableApi
@RunWith(AndroidJUnit4::class)
class VideoPlaybackTest {
    @Test
    fun extensionlessHlsRendersVideoAndSurvivesRotation() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val requests = CopyOnWriteArrayList<String>()
        val client = OkHttpClient.Builder().addInterceptor { chain ->
            val path = chain.request().url.encodedPath
            requests.add(path)
            val asset = when (path) {
                "/stream/hls/fixture" -> "playlist.m3u8"
                "/stream/hls/segment000.ts" -> "segment000.ts"
                else -> error("Unexpected media request: $path")
            }
            val bytes = instrumentation.context.assets.open("playback/$asset").use { it.readBytes() }
            val mime = if (asset.endsWith("m3u8")) "application/vnd.apple.mpegurl" else "video/mp2t"
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1)
                .code(200).message("OK").body(bytes.toResponseBody(mime.toMediaType())).build()
        }.build()
        val firstFrame = CountDownLatch(1)
        val failure = AtomicReference<PlaybackException?>()
        var player: ExoPlayer? = null

        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            try {
                scenario.onActivity { setupFrame(it) }
                waitFor(scenario) { it.findViewById<PlayerView>(R.id.player_view).player is MediaController }
                scenario.onActivity { activity ->
                    assertEquals(ActivityInfo.SCREEN_ORIENTATION_FULL_USER, activity.requestedOrientation)
                    activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
                    val exo = ExoPlayer.Builder(activity)
                        .setMediaSourceFactory(DefaultMediaSourceFactory(OkHttpDataSource.Factory(client)))
                        .build()
                    player = exo
                    exo.addListener(object : Player.Listener {
                        override fun onRenderedFirstFrame() { firstFrame.countDown() }
                        override fun onPlayerError(error: PlaybackException) { failure.set(error); firstFrame.countDown() }
                    })
                    activity.findViewById<PlayerView>(R.id.player_view).player = exo
                    exo.repeatMode = Player.REPEAT_MODE_ONE
                    exo.addListener(object : Player.Listener {
                        override fun onEvents(player: Player, events: Player.Events) { activity.embeddedPlayer!!.updateLayout() }
                    })
                    val item = PlaybackItem(42, 1, "Test channel", "Video test", null, "/stream/hls/fixture")
                    exo.setMediaItem(item.toMediaItem("https://playback.test/stream/hls/fixture"))
                    exo.prepare()
                    exo.play()
                }
                assertTrue("No video frame was rendered", firstFrame.await(15, TimeUnit.SECONDS))
                assertNull("Playback failed: ${failure.get()}", failure.get())
                assertTrue("The HLS segment was never requested", requests.contains("/stream/hls/segment000.ts"))
                waitFor(scenario) { activity ->
                    val stage = activity.findViewById<View>(R.id.player_stage)
                    activity.resources.configuration.orientation == Configuration.ORIENTATION_PORTRAIT &&
                        stage.width > 0 && kotlin.math.abs(stage.height - stage.width * 9 / 16) <= 1
                }
                scenario.onActivity { activity ->
                    assertTrue(activity.findViewById<View>(R.id.player_stage).top > 0)
                    assertTrue(player!!.videoSize.width > 0)
                    assertTrue(player!!.videoSize.height > 0)
                    activity.findViewById<PlayerView>(R.id.player_view).hideController()
                }
                screenshot("playback-portrait.png")
                scenario.onActivity { it.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE }
                waitFor(scenario) { activity ->
                    val stage = activity.findViewById<View>(R.id.player_stage)
                    val root = activity.findViewById<View>(android.R.id.content)
                    activity.resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE &&
                        stage.height == root.height && stage.width == root.width
                }
                scenario.onActivity { activity ->
                    assertSame(player, activity.findViewById<PlayerView>(R.id.player_view).player)
                }
                screenshot("playback-landscape.png")
                scenario.onActivity { it.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT }
                waitFor(scenario) { activity ->
                    val stage = activity.findViewById<View>(R.id.player_stage)
                    activity.resources.configuration.orientation == Configuration.ORIENTATION_PORTRAIT &&
                        kotlin.math.abs(stage.height - stage.width * 9 / 16) <= 1
                }
                scenario.onActivity { activity ->
                    assertSame(player, activity.findViewById<PlayerView>(R.id.player_view).player)
                    assertNull(player!!.playerError)
                    activity.embeddedPlayer!!.setFrame(JSONObject().put("mode", "mini")
                        .put("x", 120).put("y", 400).put("width", 240).put("height", 135).put("viewportWidth", 400))
                }
                waitFor(scenario) { it.findViewById<View>(R.id.player_stage).width < it.bridge.webView.width }
                screenshot("playback-mini.png")
            } finally {
                scenario.onActivity { activity ->
                    activity.findViewById<PlayerView>(R.id.player_view).player = null
                    player?.release()
                    PlaybackCoordinator.stop(activity)
                }
            }
        }
        client.dispatcher.executorService.shutdown()
        client.connectionPool.evictAll()
    }

    @Test
    fun playbackErrorShowsRetryInsteadOfABlackScreen() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { setupFrame(it) }
            waitFor(scenario) { it.findViewById<PlayerView>(R.id.player_view).player is MediaController }
            scenario.onActivity { activity ->
                // A cross-server URL is rejected by the real service before a network request.
                PlaybackCoordinator.play(activity,
                    PlaybackItem(42, 1, "Unavailable channel", null, null, "unsupported://playback.test/stream/hls/invalid"), false)
            }
            waitFor(scenario) { it.findViewById<View>(R.id.player_error_panel).visibility == View.VISIBLE }
            scenario.onActivity { activity ->
                val retry = activity.findViewById<View>(R.id.player_retry)
                assertTrue(retry.isShown)
                assertTrue(retry.performClick())
            }
            waitFor(scenario) { it.findViewById<View>(R.id.player_retry).isShown }
            scenario.onActivity { PlaybackCoordinator.stop(it) }
        }
    }

    private fun setupFrame(activity: MainActivity) {
        PlaybackCoordinator.update(PlaybackState(active = true, channelId = 42, lineupId = 1))
        activity.embeddedPlayer!!.setFrame(JSONObject().put("mode", "inline")
            .put("x", 0).put("y", 56).put("width", 400).put("height", 225).put("viewportWidth", 400))
    }

    private fun waitFor(scenario: ActivityScenario<MainActivity>, predicate: (MainActivity) -> Boolean) {
        val deadline = SystemClock.elapsedRealtime() + 10_000
        while (SystemClock.elapsedRealtime() < deadline) {
            var ready = false
            scenario.onActivity { ready = predicate(it) }
            if (ready) return
            SystemClock.sleep(50)
        }
        fail("Player did not reach the expected state")
    }

    private fun screenshot(name: String) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.waitForIdleSync()
        // The window rotation animation finishes after the new layout is measured.
        SystemClock.sleep(700)
        val bitmap = instrumentation.uiAutomation.takeScreenshot()
        File(instrumentation.targetContext.cacheDir, name).outputStream().use {
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, it)
        }
        bitmap.recycle()
    }
}
