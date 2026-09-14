package com.xivi.app

import android.Manifest
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.os.Build
import android.os.Bundle
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.activity.OnBackPressedCallback
import com.getcapacitor.BridgeActivity
import androidx.media3.common.util.UnstableApi

@UnstableApi
class MainActivity : BridgeActivity() {
    internal var embeddedPlayer: EmbeddedPlayer? = null
        private set

    override fun onCreate(savedInstanceState: Bundle?) {
        registerPlugin(XiviNativePlugin::class.java)
        super.onCreate(savedInstanceState)
        embeddedPlayer = EmbeddedPlayer(this, bridge.webView)
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                val web = bridge.webView
                if (android.net.Uri.parse(web.url.orEmpty()).path?.startsWith("/watch/channel/") == true) {
                    web.evaluateJavascript("window.dispatchEvent(new Event('xiviMinimizePlayer'))", null)
                } else if (web.canGoBack()) {
                    web.goBack()
                } else if (!PlaybackCoordinator.state().playing || embeddedPlayer?.enterPip() != true) {
                    isEnabled = false
                    onBackPressedDispatcher.onBackPressed()
                    isEnabled = true
                }
            }
        })
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1001)
        }
    }

    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        embeddedPlayer?.updateLayout()
    }

    override fun onUserLeaveHint() {
        super.onUserLeaveHint()
        embeddedPlayer?.onUserLeaveHint()
    }

    override fun onPictureInPictureModeChanged(isInPictureInPictureMode: Boolean, newConfig: Configuration) {
        super.onPictureInPictureModeChanged(isInPictureInPictureMode, newConfig)
        embeddedPlayer?.onPipChanged()
    }

    override fun onResume() {
        super.onResume()
        embeddedPlayer?.onResume()
    }

    override fun onPause() {
        embeddedPlayer?.onPause()
        super.onPause()
    }

    override fun onStop() {
        // Closing the system PiP window is an explicit dismissal of playback.
        if (isInPictureInPictureMode && !isChangingConfigurations) PlaybackCoordinator.stop(this)
        super.onStop()
    }

    override fun onDestroy() {
        embeddedPlayer?.destroy()
        embeddedPlayer = null
        super.onDestroy()
    }
}
