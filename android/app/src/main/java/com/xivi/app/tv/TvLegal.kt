@file:OptIn(androidx.tv.material3.ExperimentalTvMaterial3Api::class)

package com.xivi.app.tv

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.media3.common.util.UnstableApi
import androidx.tv.material3.Text
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import com.google.zxing.BarcodeFormat
import com.google.zxing.MultiFormatWriter

/** Uses bundled text, never a WebView or an authenticated network request. */
@UnstableApi
@Composable
internal fun AboutScreen(vm: TvViewModel) = PanelPage("About, license & source") {
    val context = LocalContext.current
    var file by remember { mutableStateOf("NOTICE.txt") }
    val document by produceState("Loading notices…", file) {
        value = withContext(Dispatchers.IO) {
            runCatching { context.assets.open("public/legal/$file").bufferedReader().use { it.readText() } }
                .getOrElse { "Bundled notice unavailable. Rebuild with npm run build:mobile before assembling the APK." }
        }
    }
    val source by produceState<Pair<String, String>?>(null) {
        value = withContext(Dispatchers.IO) {
            runCatching {
                val data = context.assets.open("public/legal/source.json").bufferedReader().use { JSONObject(it.readText()) }
                data.getString("sourceLabel") to data.getString("sourceUrl")
            }.getOrNull()
        }
    }
    val blocks = remember(document) { document.chunked(500) }
    val sourceQR by produceState<Bitmap?>(null, source) {
        val url = source?.second ?: return@produceState
        value = withContext(Dispatchers.Default) {
            runCatching {
                val matrix = MultiFormatWriter().encode(url, BarcodeFormat.QR_CODE, 256, 256)
                Bitmap.createBitmap(256, 256, Bitmap.Config.RGB_565).apply {
                    setPixels(IntArray(256 * 256) { if (matrix[it % 256, it / 256]) android.graphics.Color.BLACK else android.graphics.Color.WHITE }, 0, 256, 0, 0, 256, 256)
                }
            }.getOrNull()
        }
    }
    LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item { TvAction("Back", vm, first = true) { vm.back() } }
        item { Text("Xivi • AGPL-3.0-only • Copyright © 2023–2026 Xivi contributors", fontSize = 19.sp) }
        item { Text("You may redistribute and modify Xivi under the license. No warranty is provided, including merchantability or fitness for a particular purpose.", fontSize = 17.sp) }
        source?.let { (label, url) -> item { Text("$label:\n$url\nOpen this address on your phone or computer to access the source.", fontSize = 17.sp) } }
        sourceQR?.let { bitmap -> item { Image(bitmap.asImageBitmap(), "Scan for Xivi source code", Modifier.size(192.dp)) } }
        item { TvAction("Xivi notices", vm) { file = "NOTICE.txt" } }
        item { TvAction("Read AGPLv3", vm) { file = "LICENSE.txt" } }
        item { TvAction("Third-party software", vm) { file = "THIRD_PARTY_NOTICES.txt" } }
        item { TvAction("Web dependency notices", vm) { file = "WEB_DEPENDENCY_NOTICES.txt" } }
        // Small focusable blocks let D-pad remotes scroll the entire text.
        itemsIndexed(blocks, key = { index, _ -> "$file:$index" }) { _, text ->
            var focused by remember { mutableStateOf(false) }
            Text(text, Modifier.fillMaxWidth().onFocusChanged { focused = it.isFocused }
                .border(2.dp, if (focused) Color.White else Color.Transparent).focusable().padding(10.dp), fontSize = 17.sp)
        }
    }
}
