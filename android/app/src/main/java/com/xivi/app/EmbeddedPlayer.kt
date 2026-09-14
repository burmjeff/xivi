package com.xivi.app

import android.app.PictureInPictureParams
import android.content.ComponentName
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.graphics.Rect
import android.os.Build
import android.util.Log
import android.util.Rational
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.webkit.WebView
import android.widget.FrameLayout
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.session.MediaController
import androidx.media3.session.SessionToken
import androidx.media3.ui.PlayerView
import com.google.common.util.concurrent.ListenableFuture
import org.json.JSONObject
import kotlin.math.roundToInt

/** One video surface shared by the channel screen, in-app mini-player and Android PiP. */
@UnstableApi
class EmbeddedPlayer(private val activity: MainActivity, private val webView: WebView) {
    private val container = activity.findViewById<FrameLayout>(android.R.id.content)
    private val stage = LayoutInflater.from(activity).inflate(R.layout.embedded_player, container, false)
    internal val playerView: PlayerView = stage.findViewById(R.id.player_view)
    private var future: ListenableFuture<MediaController>? = null
    private var removeObserver: (() -> Unit)? = null
    private var frame: JSONObject? = null
    private var connectionError: String? = null
    private var destroyed = false
    private var fullscreen = false
    private val initialLightStatusBars = WindowCompat.getInsetsController(activity.window, container).isAppearanceLightStatusBars
    private val listener = object : Player.Listener {
        override fun onEvents(player: Player, events: Player.Events) = updateStatus()
    }

    init {
        container.addView(stage, FrameLayout.LayoutParams(1, 1))
        stage.visibility = View.GONE
        stage.findViewById<View>(R.id.player_retry).setOnClickListener {
            if (playerView.player == null || connectionError != null) connect()
            PlaybackCoordinator.item()?.let { PlaybackCoordinator.play(activity, it, false) }
                ?: playerView.player?.let { it.prepare(); it.play() }
        }
        stage.findViewById<View>(R.id.player_pip).setOnClickListener {
            if (!enterPip()) android.widget.Toast.makeText(activity, R.string.player_pip_unavailable, android.widget.Toast.LENGTH_LONG).show()
        }
        stage.findViewById<View>(R.id.player_pip).visibility = if (supportsPip()) View.VISIBLE else View.GONE
        container.addOnLayoutChangeListener { _, _, _, _, _, _, _, _, _ -> updateLayout() }
        removeObserver = PlaybackCoordinator.observe {
            activity.runOnUiThread { if (!destroyed) { updateStatus(); updateLayout() } }
        }
    }

    fun setFrame(value: JSONObject) {
        frame = value
        if (value.optString("mode") != "hidden" && future == null) connect()
        updateLayout()
    }

    private fun connect() {
        connectionError = null
        playerView.player?.removeListener(listener)
        playerView.player = null
        future?.let { MediaController.releaseFuture(it) }
        future = null
        try {
            val token = SessionToken(activity, ComponentName(activity, XiviPlaybackService::class.java))
            val pending = MediaController.Builder(activity, token)
                .setListener(object : MediaController.Listener {
                    override fun onDisconnected(controller: MediaController) {
                        if (destroyed || playerView.player !== controller) return
                        playerView.player = null
                        val disconnected = future
                        future = null
                        disconnected?.let { MediaController.releaseFuture(it) }
                        connectionError = activity.getString(R.string.player_connection_error)
                        updateStatus()
                    }
                }).buildAsync()
            future = pending
            pending.addListener({
                if (destroyed || future !== pending) return@addListener
                try {
                    playerView.player = pending.get().also { it.addListener(listener) }
                } catch (error: Exception) {
                    Log.e("XiviPlayback", "Could not connect embedded player", error)
                    connectionError = activity.getString(R.string.player_connection_error)
                }
                updateStatus()
            }, ContextCompat.getMainExecutor(activity))
        } catch (error: Exception) {
            Log.e("XiviPlayback", "Could not create embedded controller", error)
            connectionError = activity.getString(R.string.player_connection_error)
        }
        updateStatus()
    }

    fun updateLayout() {
        if (destroyed) return
        val pip = activity.isInPictureInPictureMode
        val active = PlaybackCoordinator.state().active
        val mode = frame?.optString("mode") ?: "hidden"
        val show = pip || (active && mode != "hidden")
        WindowCompat.getInsetsController(activity.window, container).isAppearanceLightStatusBars =
            if (show) false else initialLightStatusBars
        val expanded = pip || (show && mode == "inline" &&
            activity.resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE)
        stage.visibility = if (show) View.VISIBLE else View.GONE
        if (fullscreen != expanded) {
            fullscreen = expanded
            WindowCompat.getInsetsController(activity.window, container).apply {
                systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                if (expanded) hide(WindowInsetsCompat.Type.systemBars()) else show(WindowInsetsCompat.Type.systemBars())
            }
        }
        if (!show) { updatePipParams(); return }
        val params = if (expanded) {
            FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
        } else {
            // CSS pixels are measured against the WebView viewport, which may be inset by Capacitor.
            val value = frame ?: return
            val scale = webView.width / value.optDouble("viewportWidth", 1.0).coerceAtLeast(1.0)
            val webLocation = IntArray(2).also { webView.getLocationOnScreen(it) }
            val parentLocation = IntArray(2).also { container.getLocationOnScreen(it) }
            FrameLayout.LayoutParams(
                (value.optDouble("width") * scale).roundToInt().coerceAtLeast(1),
                (value.optDouble("height") * scale).roundToInt().coerceAtLeast(1)
            ).apply {
                leftMargin = webLocation[0] - parentLocation[0] + (value.optDouble("x") * scale).roundToInt()
                topMargin = webLocation[1] - parentLocation[1] + (value.optDouble("y") * scale).roundToInt()
            }
        }
        val old = stage.layoutParams as FrameLayout.LayoutParams
        if (old.width != params.width || old.height != params.height || old.leftMargin != params.leftMargin || old.topMargin != params.topMargin) {
            stage.layoutParams = params
        }
        playerView.useController = !pip
        stage.findViewById<View>(R.id.player_pip).visibility = if (!pip && supportsPip()) View.VISIBLE else View.GONE
        updatePipParams()
    }

    private fun updateStatus() {
        val player = playerView.player
        val error = connectionError ?: PlaybackCoordinator.state().error ?: player?.playerError?.let {
            activity.getString(R.string.player_stream_error)
        }
        stage.findViewById<View>(R.id.player_error_panel).visibility =
            if (error != null && !activity.isInPictureInPictureMode) View.VISIBLE else View.GONE
        stage.findViewById<TextView>(R.id.player_error).text = error
        stage.findViewById<View>(R.id.player_connecting).visibility =
            if (player == null && error == null) View.VISIBLE else View.GONE
        updatePipParams()
    }

    private fun supportsPip() = activity.packageManager.hasSystemFeature(PackageManager.FEATURE_PICTURE_IN_PICTURE)

    private fun shouldAutoPip() = stage.isShown && playerView.player?.let {
        it.playWhenReady && it.playbackState != Player.STATE_ENDED && it.playerError == null &&
            (it.playbackState == Player.STATE_READY || it.playbackState == Player.STATE_BUFFERING)
    } == true

    private fun pipParams(): PictureInPictureParams {
        val bounds = Rect()
        val builder = PictureInPictureParams.Builder().setAspectRatio(Rational(16, 9))
        if (stage.getGlobalVisibleRect(bounds) && bounds.width() > 0 && bounds.height() > 0) builder.setSourceRectHint(bounds)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            builder.setAutoEnterEnabled(shouldAutoPip()).setSeamlessResizeEnabled(true)
        }
        return builder.build()
    }

    private fun updatePipParams() {
        if (supportsPip()) activity.setPictureInPictureParams(pipParams())
    }

    fun enterPip(): Boolean {
        if (!supportsPip() || !stage.isShown || playerView.player == null) return false
        return runCatching { activity.enterPictureInPictureMode(pipParams()) }.getOrDefault(false)
    }

    fun onUserLeaveHint() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S && shouldAutoPip()) enterPip()
    }

    fun onPipChanged() { updateLayout(); updateStatus() }
    fun onResume() = playerView.onResume()
    fun onPause() { if (!activity.isInPictureInPictureMode) playerView.onPause() }

    fun destroy() {
        destroyed = true
        removeObserver?.invoke()
        playerView.player?.removeListener(listener)
        playerView.player = null
        future?.let { MediaController.releaseFuture(it) }
        container.removeView(stage)
    }
}
