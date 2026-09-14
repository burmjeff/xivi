package com.xivi.app

import android.content.ComponentName
import android.content.Context
import android.os.Looper
import androidx.media3.common.util.UnstableApi
import androidx.media3.session.MediaController
import androidx.media3.session.SessionToken
import androidx.media3.ui.PlayerView
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.google.common.util.concurrent.ListenableFuture
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.TimeUnit

@UnstableApi
@RunWith(AndroidJUnit4::class)
class PlaybackSessionTest {
    @Test
    fun playbackServiceResolvesAndConnects() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        // Exercise Android's installed manifest lookup, just as PlayerActivity does.
        val token = SessionToken(context, ComponentName(context, XiviPlaybackService::class.java))
        assertEquals(SessionToken.TYPE_SESSION_SERVICE, token.type)

        val instrumentation = InstrumentationRegistry.getInstrumentation()
        lateinit var future: ListenableFuture<MediaController>
        instrumentation.runOnMainSync {
            future = MediaController.Builder(context, token)
                .setApplicationLooper(Looper.getMainLooper())
                .buildAsync()
        }
        try {
            val controller = future.get(10, TimeUnit.SECONDS)
            instrumentation.runOnMainSync {
                assertTrue(controller.isConnected)
            }
        } finally {
            instrumentation.runOnMainSync { MediaController.releaseFuture(future) }
        }
    }

    @Test
    fun playerActivityLaunchesWithoutCrashing() {
        ActivityScenario.launch(PlayerActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                assertNotNull(activity.findViewById<PlayerView>(R.id.player_view))
            }
        }
    }
}
