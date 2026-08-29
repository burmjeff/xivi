package com.xivi.app

import android.app.PictureInPictureParams
import android.content.BroadcastReceiver
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
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

    private val carReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action == PlaybackCoordinator.ACTION_CAR_CONNECTED) finishAndRemoveTask()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
        setContentView(R.layout.activity_player)
        playerView = findViewById(R.id.player_view)
        window.insetsController?.apply {
            hide(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
            systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        }
        ContextCompat.registerReceiver(
            this, carReceiver, IntentFilter(PlaybackCoordinator.ACTION_CAR_CONNECTED),
            ContextCompat.RECEIVER_NOT_EXPORTED
        )
        val token = SessionToken(this, ComponentName(this, XiviMediaLibraryService::class.java))
        controllerFuture = MediaController.Builder(this, token).buildAsync().also { future ->
            future.addListener(
                { runCatching { playerView.player = future.get() } },
                ContextCompat.getMainExecutor(this)
            )
        }
    }

    override fun onResume() {
        super.onResume()
        if (PlaybackCoordinator.state().audioOnly) finishAndRemoveTask()
    }

    override fun onUserLeaveHint() {
        val player = playerView.player
        if (player != null && player.playWhenReady && !PlaybackCoordinator.state().audioOnly) {
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
        unregisterReceiver(carReceiver)
        playerView.player = null
        controllerFuture?.let { MediaController.releaseFuture(it) }
        super.onDestroy()
    }
}
