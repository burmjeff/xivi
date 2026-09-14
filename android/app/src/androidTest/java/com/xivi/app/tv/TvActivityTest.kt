package com.xivi.app.tv

import android.view.View
import android.view.ViewGroup
import android.webkit.WebView
import androidx.media3.common.util.UnstableApi
import androidx.test.core.app.ActivityScenario
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Test

@UnstableApi
class TvActivityTest {
    @Test fun licenseAndSourceNoticesAreBundledOffline() {
        val assets = InstrumentationRegistry.getInstrumentation().targetContext.assets
        val license = assets.open("public/legal/LICENSE.txt").bufferedReader().use { it.readText() }
        assertTrue(license.contains("GNU AFFERO GENERAL PUBLIC LICENSE"))
        assertTrue(license.contains("END OF TERMS AND CONDITIONS"))
        val source = assets.open("public/legal/source.json").bufferedReader().use { org.json.JSONObject(it.readText()) }
        assertTrue(source.getString("sourceUrl").startsWith("https://"))
        val dependencies = assets.open("public/legal/WEB_DEPENDENCY_NOTICES.txt").bufferedReader().use { it.readText() }
        assertTrue(dependencies.contains("SIL OPEN FONT LICENSE"))
    }
    @Test fun leanbackEntryIsNativeAndHasNoPip() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val launch = context.packageManager.getLeanbackLaunchIntentForPackage(context.packageName)
        assertEquals(TvActivity::class.java.name, launch?.component?.className)
        fun containsWebView(view: View): Boolean = view is WebView ||
            (view is ViewGroup && (0 until view.childCount).any { containsWebView(view.getChildAt(it)) })
        ActivityScenario.launch(TvActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                assertFalse("TV launch instantiated a WebView", containsWebView(activity.window.decorView))
                assertFalse(activity.isInPictureInPictureMode)
                activity.onBackPressedDispatcher.onBackPressed()
                assertTrue("Back at the unpaired playback root must finish", activity.isFinishing)
            }
        }
    }
}
