package com.xivi.app

import android.app.PictureInPictureParams
import android.content.ComponentName
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.os.Bundle
import android.util.Log
import android.util.Rational
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.session.MediaController
import androidx.media3.session.SessionToken
import androidx.media3.ui.PlayerView
import com.google.common.util.concurrent.ListenableFuture

@UnstableApi
class PlayerActivity : AppCompatActivity() {
    private lateinit var root: View
    private lateinit var playerView: PlayerView
    private var controllerFuture: ListenableFuture<MediaController>? = null
    private var removePlaybackObserver: (() -> Unit)? = null
    private var connectionError: String? = null
    private var serviceError: String? = null
    private val playerListener = object : Player.Listener {
        override fun onEvents(player: Player, events: Player.Events) = updateStatus()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, false)
        setContentView(R.layout.activity_player)
        root = findViewById(R.id.player_root)
        playerView = findViewById(R.id.player_view)
        findViewById<View>(R.id.player_back).setOnClickListener { finish() }
        findViewById<View>(R.id.player_retry).setOnClickListener { retryPlayback() }
        ViewCompat.setOnApplyWindowInsetsListener(root) { view, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
            if (isFullscreen()) view.setPadding(0, 0, 0, 0)
            else view.setPadding(bars.left, bars.top, bars.right, bars.bottom)
            resizeStage()
            insets
        }
        root.addOnLayoutChangeListener { _, left, _, right, _, oldLeft, _, oldRight, _ ->
            if (right - left != oldRight - oldLeft) resizeStage()
        }
        updatePresentation()
        removePlaybackObserver = PlaybackCoordinator.observe { state ->
            runOnUiThread {
                if (!isDestroyed) {
                    serviceError = state.error
                    updateStatus()
                }
            }
        }
        connectController()
    }

    private fun connectController() {
        connectionError = null
        playerView.player?.removeListener(playerListener)
        playerView.player = null
        controllerFuture?.let { MediaController.releaseFuture(it) }
        controllerFuture = null
        updateStatus()
        try {
            val token = SessionToken(this, ComponentName(this, XiviPlaybackService::class.java))
            val future = MediaController.Builder(this, token).buildAsync()
            controllerFuture = future
            future.addListener({
                if (isDestroyed || controllerFuture !== future) return@addListener
                try {
                    val controller = future.get()
                    playerView.player = controller
                    controller.addListener(playerListener)
                } catch (error: Exception) {
                    Log.e("XiviPlayback", "Could not connect the player", error)
                    connectionError = getString(R.string.player_connection_error)
                }
                updateStatus()
            }, ContextCompat.getMainExecutor(this))
        } catch (error: Exception) {
            Log.e("XiviPlayback", "Could not create the playback controller", error)
            connectionError = getString(R.string.player_connection_error)
            updateStatus()
        }
    }

    private fun retryPlayback() {
        try {
            if (connectionError != null || playerView.player == null) connectController()
            val item = PlaybackCoordinator.item()
            if (item != null) PlaybackCoordinator.play(this, item, openPlayer = false)
            else playerView.player?.let { player ->
                if (player.playerError?.errorCode == PlaybackException.ERROR_CODE_BEHIND_LIVE_WINDOW) {
                    player.seekToDefaultPosition()
                }
                player.prepare()
                player.play()
            }
        } catch (error: Exception) {
            Log.e("XiviPlayback", "Could not retry playback", error)
            connectionError = getString(R.string.player_stream_error)
        }
        updateStatus()
    }

    private fun updateStatus() {
        val player = playerView.player
        val item = PlaybackCoordinator.item()
        findViewById<TextView>(R.id.player_title).text = player?.mediaMetadata?.title ?: item?.name ?: "Live channel"
        findViewById<TextView>(R.id.player_programme).text = player?.mediaMetadata?.subtitle ?: item?.programme ?: "Live"
        val error = connectionError ?: serviceError ?: player?.playerError?.let { getString(R.string.player_stream_error) }
        findViewById<View>(R.id.player_error_panel).visibility = if (error != null && !isInPictureInPictureMode) View.VISIBLE else View.GONE
        findViewById<TextView>(R.id.player_error).text = error
        findViewById<View>(R.id.player_connecting).visibility = if (player == null && error == null) View.VISIBLE else View.GONE
    }

    private fun isFullscreen() =
        resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE || isInPictureInPictureMode

    private fun resizeStage() {
        val stage = findViewById<View>(R.id.player_stage)
        val width = root.width - root.paddingLeft - root.paddingRight
        val height = if (isFullscreen()) ViewGroup.LayoutParams.MATCH_PARENT else (width * 9 / 16).coerceAtLeast(1)
        if (stage.layoutParams.height != height) {
            stage.layoutParams = stage.layoutParams.apply { this.height = height }
        }
    }

    private fun updatePresentation() {
        val fullscreen = isFullscreen()
        findViewById<View>(R.id.player_toolbar).visibility = if (fullscreen) View.GONE else View.VISIBLE
        findViewById<View>(R.id.player_details).visibility = if (fullscreen) View.GONE else View.VISIBLE
        WindowCompat.getInsetsController(window, root).apply {
            isAppearanceLightStatusBars = false
            isAppearanceLightNavigationBars = false
            systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            if (fullscreen) hide(WindowInsetsCompat.Type.systemBars()) else show(WindowInsetsCompat.Type.systemBars())
        }
        playerView.useController = !isInPictureInPictureMode
        resizeStage()
        ViewCompat.requestApplyInsets(root)
    }

    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        updatePresentation()
    }

    override fun onResume() {
        super.onResume()
        playerView.onResume()
    }

    override fun onPause() {
        playerView.onPause()
        super.onPause()
    }

    override fun onUserLeaveHint() {
        super.onUserLeaveHint()
        if (playerView.player?.isPlaying == true && packageManager.hasSystemFeature(PackageManager.FEATURE_PICTURE_IN_PICTURE)) {
            enterPictureInPictureMode(PictureInPictureParams.Builder().setAspectRatio(Rational(16, 9)).build())
        }
    }

    override fun onPictureInPictureModeChanged(isInPictureInPictureMode: Boolean, newConfig: Configuration) {
        super.onPictureInPictureModeChanged(isInPictureInPictureMode, newConfig)
        updatePresentation()
        updateStatus()
    }

    override fun onDestroy() {
        removePlaybackObserver?.invoke()
        playerView.player?.removeListener(playerListener)
        playerView.player = null
        controllerFuture?.let { MediaController.releaseFuture(it) }
        controllerFuture = null
        super.onDestroy()
    }
}
