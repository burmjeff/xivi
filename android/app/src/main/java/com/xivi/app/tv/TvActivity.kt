package com.xivi.app.tv

import android.os.Bundle
import android.view.KeyEvent
import android.view.WindowManager
import androidx.appcompat.app.AppCompatActivity
import androidx.activity.addCallback
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.media3.common.util.UnstableApi

/** The Leanback entry point never constructs Capacitor or a WebView. */
@UnstableApi
class TvActivity : AppCompatActivity() {
    private val model by viewModels<TvViewModel>()
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, false)
        WindowInsetsControllerCompat(window, window.decorView).hide(WindowInsetsCompat.Type.systemBars())
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        onBackPressedDispatcher.addCallback(this) { if (!model.back()) finish() }
        setContent { TvApp(model) }
    }
    override fun onStart() { super.onStart(); model.onStart() }
    override fun onStop() { model.onStop(); super.onStop() }
    override fun dispatchKeyEvent(event: KeyEvent): Boolean = model.key(event) || super.dispatchKeyEvent(event)
}
