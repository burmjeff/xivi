package com.xivi.app

import android.app.PictureInPictureParams
import android.content.ComponentName
import android.content.pm.ActivityInfo
import android.os.Bundle
import android.util.Rational
import android.view.WindowInsets
import android.view.WindowInsetsController
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.media3.common.util.UnstableApi
import androidx.media3.session.MediaController
import androidx.media3.session.SessionToken
import androidx.media3.ui.PlayerView
import com.google.common.util.concurrent.ListenableFuture

@UnstableApi
class PlayerActivity : AppCompatActivity() {
    private lateinit var playerView: PlayerView
    private var controllerFuture: ListenableFuture<MediaController>? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
        setContentView(R.layout.activity_player)
        playerView = findViewById(R.id.player_view)
        window.insetsController?.apply {
            hide(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
            systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        }
        val token = SessionToken(this, ComponentName(this, XiviPlaybackService::class.java))
        controllerFuture = MediaController.Builder(this, token).buildAsync().also { future ->
            future.addListener(
                { runCatching { playerView.player = future.get() } },
                ContextCompat.getMainExecutor(this)
            )
        }
    }

    override fun onUserLeaveHint() {
        val player = playerView.player
        if (player != null && player.playWhenReady) {
            enterPictureInPictureMode(
                PictureInPictureParams.Builder().setAspectRatio(Rational(16, 9)).build()
            )
        }
    }

    override fun onPictureInPictureModeChanged(isInPictureInPictureMode: Boolean) {
        super.onPictureInPictureModeChanged(isInPictureInPictureMode)
        playerView.useController = !isInPictureInPictureMode
    }

    override fun onDestroy() {
        playerView.player = null
        controllerFuture?.let { MediaController.releaseFuture(it) }
        super.onDestroy()
    }
}
