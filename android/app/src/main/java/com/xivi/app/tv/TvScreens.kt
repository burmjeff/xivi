@file:OptIn(androidx.tv.material3.ExperimentalTvMaterial3Api::class)

package com.xivi.app.tv

import android.graphics.Bitmap
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.*
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.key.*
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.*
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.media3.common.C
import androidx.media3.common.util.UnstableApi
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.CaptionStyleCompat
import androidx.media3.ui.PlayerView
import androidx.tv.material3.*
import com.google.zxing.BarcodeFormat
import com.google.zxing.MultiFormatWriter
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val LocalHighContrast = staticCompositionLocalOf { false }
private val Canvas: Color @Composable get() = if (LocalHighContrast.current) Color.Black else Color(0xff0c141d)
private val Panel: Color @Composable get() = if (LocalHighContrast.current) Color.Black else Color(0xff172532)
private val Aqua = Color(0xff70e3cd)
private val Muted: Color @Composable get() = if (LocalHighContrast.current) Color.White else Color(0xffb5c3cc)
private fun guideRows(settings: TvSettings, fontScale: Float) = when {
    fontScale > 2.5f -> 1
    fontScale > 1.7f -> 3
    fontScale > 1.25f -> minOf(settings.density.rows, 5)
    else -> settings.density.rows
}
private fun guideHeaderHeight(settings: TvSettings, fontScale: Float) =
    (if (fontScale > 1.7f) 112f else (if (settings.density == GuideDensity.COMPACT || fontScale > 1.25f) 80 else 104) * fontScale.coerceAtLeast(1f)).dp
private fun timeLabel(time: Long) = Instant.ofEpochMilli(time).atZone(ZoneId.systemDefault()).format(DateTimeFormatter.ofPattern("h:mm a"))
private fun dateLabel(time: Long) = Instant.ofEpochMilli(time).atZone(ZoneId.systemDefault()).format(DateTimeFormatter.ofPattern("EEE, MMM d"))

@UnstableApi
@Composable
fun TvApp(vm: TvViewModel) {
    val settings by vm.settings.collectAsStateWithLifecycle()
    val surface by vm.surface.collectAsStateWithLifecycle()
    val auth by vm.auth.state.collectAsStateWithLifecycle()
    val status by vm.playback.status.collectAsStateWithLifecycle()
    val message by vm.message.collectAsStateWithLifecycle()
    val banner by vm.banner.collectAsStateWithLifecycle()
    val digits by vm.numberInput.collectAsStateWithLifecycle()
    val density = LocalDensity.current
    val screenState = rememberSaveableStateHolder()
    MaterialTheme(colorScheme = darkColorScheme(primary = Aqua, background = Canvas, surface = Panel, onSurface = Color.White)) {
        CompositionLocalProvider(LocalDensity provides Density(density.density, density.fontScale * settings.textScale), LocalHighContrast provides settings.highContrast, LocalContentColor provides Color.White) {
            Box(Modifier.fillMaxSize().background(Color.Black)) {
                val preview = surface == TvSurface.GUIDE || (surface == TvSurface.MINI && !settings.miniOverlay)
                val previewHeight = guideHeaderHeight(settings, LocalDensity.current.fontScale)
                val playerModifier = if (preview) Modifier.align(Alignment.TopEnd).padding(top = 8.dp, end = 32.dp).width(previewHeight * (16f / 9f)).height(previewHeight)
                    else Modifier.fillMaxSize()
                // One AndroidView and surface, even when the guide resizes it.
                AndroidView(factory = { context -> PlayerView(context).apply { player = vm.playback.player; useController = false; isFocusable = false; descendantFocusability = android.view.ViewGroup.FOCUS_BLOCK_DESCENDANTS } },
                    modifier = playerModifier, update = { view ->
                        val effective = vm.effectiveSettings()
                        view.resizeMode = when (effective.aspect) { "zoom" -> AspectRatioFrameLayout.RESIZE_MODE_ZOOM; "stretch" -> AspectRatioFrameLayout.RESIZE_MODE_FILL; else -> AspectRatioFrameLayout.RESIZE_MODE_FIT }
                        view.subtitleView?.setFractionalTextSize(0.0533f * settings.captionScale)
                        if (settings.highContrast) view.subtitleView?.setStyle(CaptionStyleCompat(android.graphics.Color.WHITE, android.graphics.Color.BLACK, android.graphics.Color.TRANSPARENT, CaptionStyleCompat.EDGE_TYPE_OUTLINE, android.graphics.Color.BLACK, null))
                        else view.subtitleView?.setUserDefaultStyle()
                    })
                if (surface == TvSurface.ABOUT) {
                    AboutScreen(vm)
                } else if (auth in listOf(TvAuthState.UNCONFIGURED, TvAuthState.UNPAIRED, TvAuthState.REVOKED)) {
                    Onboarding(vm, auth)
                } else {
                    screenState.SaveableStateProvider(surface.name) { when (surface) {
                        TvSurface.LIVE -> {
                            if (status.channel == null) Column(Modifier.align(Alignment.Center).background(Canvas).padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                                Text("Xivi • Live TV", fontSize = 28.sp)
                                TvAction("Open guide", vm, first = true) { vm.show(TvSurface.GUIDE) }
                                TvAction("Reconnect", vm) { vm.connect() }
                                TvAction("Settings", vm) { vm.show(TvSurface.SETTINGS) }
                            }
                            if (banner != null) ChannelBanner(vm, banner!!, Modifier.align(Alignment.BottomCenter))
                            if (status.loading) Text("Tuning ${status.channel?.name.orEmpty()}…", Modifier.align(Alignment.Center).background(Canvas.copy(alpha = .85f)).padding(18.dp), fontSize = 21.sp)
                            if (status.message.isNotBlank()) Text(status.message, Modifier.align(Alignment.TopStart).padding(32.dp).background(Canvas).padding(12.dp), fontSize = 18.sp)
                        }
                        TvSurface.GUIDE, TvSurface.MINI -> GuideScreen(vm, surface == TvSurface.MINI)
                        TvSurface.QUICK -> QuickControls(vm)
                        TvSurface.FILTERS -> FilterScreen(vm)
                        TvSurface.FAVORITES -> FavoritesScreen(vm)
                        TvSurface.DATE -> DateScreen(vm)
                        TvSurface.DETAILS -> DetailsScreen(vm)
                        TvSurface.RECENTS -> RecentScreen(vm)
                        TvSurface.SEARCH -> SearchScreen(vm)
                        TvSurface.SETTINGS, TvSurface.PLAYBACK -> SettingsScreen(vm, surface == TvSurface.PLAYBACK)
                        TvSurface.CHANNELS -> ChannelEditor(vm)
                        TvSurface.HELP -> ControlsHelp(vm)
                        TvSurface.AUDIO, TvSurface.CAPTIONS -> TrackScreen(vm, surface == TvSurface.CAPTIONS)
                        TvSurface.ABOUT -> Unit // Rendered above, also available before pairing.
                    } }
                }
                if (digits.isNotBlank()) Text(digits, Modifier.align(Alignment.TopEnd).padding(32.dp).background(Canvas).padding(20.dp), fontSize = 40.sp)
                if (message.isNotBlank()) Row(Modifier.align(Alignment.BottomCenter).fillMaxWidth().background(Canvas).padding(horizontal = 28.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(message, Modifier.weight(1f), fontSize = 14.sp, maxLines = 2)
                    Button(onClick = { vm.message.value = "" }, scale = ButtonDefaults.scale(focusedScale = 1f)) { Text("Dismiss", fontSize = 14.sp) }
                }
            }
        }
    }
}

@UnstableApi
@Composable
internal fun TvAction(label: String, vm: TvViewModel, modifier: Modifier = Modifier, first: Boolean = false, onClick: () -> Unit) {
    val requester = remember { FocusRequester() }
    val screen by vm.surface.collectAsStateWithLifecycle()
    val initial = remember { vm.menuFocus[screen] ?: if (first) label else "" }
    val settings by vm.settings.collectAsStateWithLifecycle()
    var focused by remember { mutableStateOf(false) }
    if (settings.reducedMotion) {
        Box(modifier.focusRequester(requester).onFocusChanged { focused = it.isFocused; if (it.isFocused) vm.menuFocus[screen] = label }
            .border(if (focused) 3.dp else 1.dp, if (focused) Color.White else Muted, RoundedCornerShape(8.dp))
            .background(if (focused) Color(0xff31485a) else Panel).clickable(role = Role.Button, onClick = onClick).padding(14.dp)) {
            Text(label, fontSize = 17.sp, maxLines = 3, overflow = TextOverflow.Ellipsis)
        }
    } else {
    Button(onClick = onClick, modifier = modifier.focusRequester(requester).onFocusChanged { if (it.isFocused) vm.menuFocus[screen] = label },
        scale = ButtonDefaults.scale(focusedScale = 1f), border = ButtonDefaults.border(focusedBorder = Border(BorderStroke(3.dp, Color.White))),
        colors = ButtonDefaults.colors(containerColor = Panel, focusedContainerColor = Color(0xff31485a), focusedContentColor = Color.White)) {
        Text(label, fontSize = 17.sp, maxLines = 3, overflow = TextOverflow.Ellipsis)
    }
    }
    LaunchedEffect(Unit) { if (initial == label) requester.requestFocus() }
}

@Composable
private fun Field(label: String, value: String, change: (String) -> Unit, modifier: Modifier = Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(label, color = Muted, fontSize = 16.sp)
        var focused by remember { mutableStateOf(false) }
        BasicTextField(value, change, Modifier.fillMaxWidth().onFocusChanged { focused = it.isFocused }.border(if (focused) 3.dp else 1.dp, if (focused) Color.White else Muted, RoundedCornerShape(8.dp)).padding(14.dp),
            textStyle = TextStyle(color = Color.White, fontSize = 20.sp), singleLine = true, cursorBrush = SolidColor(Aqua))
    }
}

@UnstableApi
@Composable
private fun Onboarding(vm: TvViewModel, state: TvAuthState) {
    val pairing by vm.pairing.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()
    var server by remember { mutableStateOf(vm.auth.server().orEmpty()) }
    Row(Modifier.fillMaxSize().background(Canvas).padding(48.dp), horizontalArrangement = Arrangement.spacedBy(48.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Text("Xivi", color = Aqua, fontSize = 42.sp, fontWeight = FontWeight.Bold)
            Text(if (state == TvAuthState.REVOKED) "Pair this TV again" else "Your channels, on the big screen", fontSize = 26.sp)
            if (pairing == null) {
                Text("Connect to one trusted HTTPS Xivi server. Then approve this TV on your phone—no password typing on the remote.", color = Muted, fontSize = 18.sp)
                Field("Xivi server", server, { server = it })
                TvAction(if (busy) "Connecting…" else "Connect server", vm, first = true) { if (!busy) vm.configureServer(server) }
                if (vm.auth.server() != null) TvAction("Get a pairing code", vm) { vm.beginPairing() }
            } else {
                Text("1. Scan the QR code or visit", fontSize = 19.sp)
                Text(pairing!!.getString("verification_uri"), color = Aqua, fontSize = 19.sp)
                Text("2. Sign in on your phone and approve the matching TV code.", fontSize = 19.sp)
                Text(pairing!!.getString("user_code"), fontSize = 38.sp, fontWeight = FontWeight.Bold)
                Text("Code expires after 10 minutes. Your paired TV stays signed in until revoked.", color = Muted, fontSize = 16.sp)
                TvAction("Request new code", vm, first = true) { vm.beginPairing() }
            }
            TvAction("About, license & source", vm) { vm.show(TvSurface.ABOUT) }
        }
        pairing?.getString("verification_uri_complete")?.let { approvalURL ->
            val bitmap by produceState<Bitmap?>(null, approvalURL) {
                value = withContext(Dispatchers.Default) {
                    val matrix = MultiFormatWriter().encode(approvalURL, BarcodeFormat.QR_CODE, 256, 256)
                    Bitmap.createBitmap(256, 256, Bitmap.Config.RGB_565).apply { setPixels(IntArray(256 * 256) { if (matrix[it % 256, it / 256]) android.graphics.Color.BLACK else android.graphics.Color.WHITE }, 0, 256, 0, 0, 256, 256) }
                }
            }
            bitmap?.let { Image(it.asImageBitmap(), "Scan to approve this TV", Modifier.size(256.dp).background(Color.White)) }
        }
    }
}

@UnstableApi
@Composable
private fun ChannelLogo(vm: TvViewModel, channel: TvChannel, size: Int = 36) {
    val bitmap by produceState<Bitmap?>(null, channel.logo, vm.auth.cacheNamespace()) { value = vm.repository.logo(channel) }
    bitmap?.let { Image(it.asImageBitmap(), null, Modifier.size(size.dp)) } ?: Spacer(Modifier.size(size.dp))
}

@UnstableApi
@Composable
private fun ChannelBanner(vm: TvViewModel, channel: TvChannel, modifier: Modifier = Modifier) {
    val now by vm.clock.collectAsStateWithLifecycle()
    val p = channel.at(now)
    Row(modifier.fillMaxWidth().background(Canvas.copy(alpha = .96f)).padding(24.dp), horizontalArrangement = Arrangement.spacedBy(20.dp), verticalAlignment = Alignment.CenterVertically) {
        ChannelLogo(vm, channel, 52)
        Column(Modifier.weight(1f)) {
            Text("${channel.number}  ${channel.name}", fontSize = 24.sp, fontWeight = FontWeight.Bold)
            Text(p?.title ?: "No programme information", fontSize = 20.sp)
            if (p != null) { Text("${timeLabel(p.start)} – ${timeLabel(p.end)}", color = Muted, fontSize = 15.sp); Progress(p, now) }
        }
        Text("LIVE", color = Aqua, fontWeight = FontWeight.Bold)
    }
}

@Composable
private fun Progress(programme: TvProgramme, now: Long) {
    val fraction = ((now - programme.start).toFloat() / (programme.end - programme.start)).coerceIn(0f, 1f)
    Box(Modifier.fillMaxWidth().padding(top = 5.dp).height(3.dp).background(Color.Gray)) { Box(Modifier.fillMaxWidth(fraction).fillMaxHeight().background(Aqua)) }
}

@UnstableApi
@Composable
private fun GuideScreen(vm: TvViewModel, mini: Boolean) {
    val all by vm.repository.channels.collectAsStateWithLifecycle()
    val prefs by vm.repository.preferences.collectAsStateWithLifecycle()
    val settings by vm.settings.collectAsStateWithLifecycle()
    val selected by vm.focus.collectAsStateWithLifecycle()
    val playback by vm.playback.status.collectAsStateWithLifecycle()
    val now by vm.clock.collectAsStateWithLifecycle()
    val updated by vm.repository.guideUpdatedAt.collectAsStateWithLifecycle()
    val category by vm.repository.categoryKeys.collectAsStateWithLifecycle()
    val fontScale = LocalDensity.current.fontScale
    val rows = remember(all, prefs, settings, category) { vm.channels() }
    val selectedChannel = all.find { it.key == selected.channelKey }
    val programme = selectedChannel?.at(selected.time)
    val focusRequester = remember { FocusRequester() }
    var gridFocused by remember { mutableStateOf(false) }
    val index = rows.indexOfFirst { it.key == selected.channelKey }.coerceAtLeast(0)
    val count = if (mini) { if (fontScale > 2.5f) 1 else if (fontScale > 1.7f) 2 else 3 } else guideRows(settings, fontScale)
    val top = (index - count / 2).coerceIn(0, (rows.size - count).coerceAtLeast(0))
    var windowStart by remember { mutableLongStateOf(Math.floorDiv(selected.time, 30 * 60_000L) * 30 * 60_000L) }
    val duration = settings.density.minutes * 60_000L
    LaunchedEffect(selected.time) { if (selected.time < windowStart || selected.time >= windowStart + duration) windowStart = Math.floorDiv(selected.time, 30 * 60_000L) * 30 * 60_000L }
    val guideModifier = if (mini) Modifier.fillMaxSize().padding(top = if (settings.miniOverlay) 190.dp else guideHeaderHeight(settings, fontScale) + 16.dp) else Modifier.fillMaxSize()
    Column(guideModifier.background(if (mini) Canvas.copy(alpha = .96f) else Color.Transparent).padding(horizontal = 32.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        if (!mini) {
            Row(Modifier.fillMaxWidth().height(guideHeaderHeight(settings, fontScale))) {
                Column(Modifier.weight(1f).background(Canvas).padding(6.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    if (fontScale <= 2.2f) Text(selectedChannel?.let { "${it.number}  ${it.name}" } ?: "Channel guide", color = Aqua, fontSize = 16.sp, maxLines = 1)
                    Text(programme?.title ?: "No programme information", fontSize = 20.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    if (settings.showDetails && fontScale <= 1.7f) {
                        Text(programme?.let { "${timeLabel(it.start)} – ${timeLabel(it.end)}  ${it.subtitle}" } ?: "You can still watch this channel.", color = Muted, fontSize = 15.sp, maxLines = 1)
                        if (settings.density != GuideDensity.COMPACT && fontScale <= 1.25f) Text(programme?.description.orEmpty(), fontSize = 14.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
                Spacer(Modifier.width(guideHeaderHeight(settings, fontScale) * (16f / 9f) + 16.dp))
            }
        }
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
            TvAction("Filters", vm, Modifier.height((36 * fontScale.coerceAtLeast(1f)).dp)) { vm.show(TvSurface.FILTERS) }
            TvAction("Now", vm, Modifier.height((36 * fontScale.coerceAtLeast(1f)).dp)) { vm.now(); focusRequester.requestFocus() }
            if (!mini) {
                TvAction("Day −", vm, Modifier.height((36 * fontScale.coerceAtLeast(1f)).dp)) { vm.day(-1); focusRequester.requestFocus() }
                TvAction("Day +", vm, Modifier.height((36 * fontScale.coerceAtLeast(1f)).dp)) { vm.day(1); focusRequester.requestFocus() }
                TvAction("Date", vm, Modifier.height((36 * fontScale.coerceAtLeast(1f)).dp)) { vm.show(TvSurface.DATE) }
            }
            TvAction(if (mini) "Full guide" else "Search", vm, Modifier.height((36 * fontScale.coerceAtLeast(1f)).dp)) { vm.show(if (mini) TvSurface.GUIDE else TvSurface.SEARCH) }
            TvAction("Close", vm, Modifier.height((36 * fontScale.coerceAtLeast(1f)).dp)) { vm.back() }
        }
        Text("${dateLabel(selected.time)}  •  ${rows.size} channels" + (if (count != settings.density.rows && !mini) "  •  Large text layout" else "") + if (updated == 0L || now - updated > 120_000) "  •  Saved listings" else "", color = Muted, fontSize = 13.sp, maxLines = 1)
        Column(Modifier.weight(1f).fillMaxWidth().background(Canvas).focusRequester(focusRequester).onFocusChanged { gridFocused = it.isFocused }
            .onPreviewKeyEvent { event ->
                if (event.type != KeyEventType.KeyDown) false else when (event.key) {
                    Key.DirectionUp -> vm.moveGuide(-1)
                    Key.DirectionDown -> vm.moveGuide(1)
                    Key.DirectionLeft -> { vm.moveTime(-1); true }
                    Key.DirectionRight -> { vm.moveTime(1); true }
                    Key.DirectionCenter, Key.Enter -> { vm.select(); true }
                    else -> false
                }
            }.focusable().semantics { contentDescription = "Channel guide. Up and down change rows; left and right change programme time. OK selects. ${selectedChannel?.name.orEmpty()}, ${programme?.title ?: "No programme information"}" }) {
            if (rows.isEmpty()) Text("No channels in this list. Change Filters to see other channels.", Modifier.padding(20.dp))
            if (!mini) Row(Modifier.height((24 * fontScale.coerceAtLeast(1f)).dp)) {
                Spacer(Modifier.width(180.dp))
                repeat(settings.density.minutes / 30) { slot -> Text(timeLabel(windowStart + slot * 30 * 60_000L), Modifier.weight(1f), color = Muted, fontSize = 13.sp) }
            }
            for (channel in rows.drop(top).take(count)) {
                key(channel.key) {
                    val focused = channel.key == selected.channelKey
                    Row(Modifier.weight(1f).fillMaxWidth().padding(vertical = 1.dp)
                        .semantics { testTag = "tv-row-${channel.key}" }
                        .border(if (focused && gridFocused) 3.dp else 1.dp, if (focused && gridFocused) Color.White else Color(0xff2e414f))) {
                        Row(Modifier.width(180.dp).fillMaxHeight().background(Panel).padding(6.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                            ChannelLogo(vm, channel, 28)
                            if ((settings.density == GuideDensity.COMPACT || fontScale > 1.25f) && !mini) {
                                Text("${channel.number} ${if (prefs.favorite(channel.key)) "★" else ""} ${if (channel.key == playback.channel?.key) "▶" else ""} ${channel.name}", Modifier.weight(1f), fontSize = 14.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, color = if (channel.key == playback.channel?.key) Aqua else Color.White)
                            } else Column(Modifier.weight(1f)) {
                                Text("${channel.number} ${if (prefs.favorite(channel.key)) "★" else ""} ${if (channel.key == playback.channel?.key) "▶ LIVE" else ""}", fontSize = 12.sp, color = if (channel.key == playback.channel?.key) Aqua else Muted)
                                Text(channel.name, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }
                        if (mini) {
                            Column(Modifier.weight(1f).padding(8.dp)) {
                                val current = channel.at(selected.time)
                                Text(current?.title ?: "No programme information", fontSize = 18.sp, maxLines = 1)
                                Text("Next: ${channel.next(selected.time)?.title ?: "No programme information"}", color = Muted, fontSize = 14.sp, maxLines = 1)
                                if (current != null) Progress(current, now)
                            }
                        } else ScheduleCells(channel, selected, windowStart, duration, now, focused && gridFocused) { instant -> vm.focus.value = GuideFocus(channel.key, instant); vm.select() }
                    }
                }
            }
        }
        if (fontScale <= 1.7f) Text("↑↓ Channels   ←→ Programmes   OK Select   Back Close", color = Muted, fontSize = 13.sp)
    }
    LaunchedEffect(Unit) { focusRequester.requestFocus() }
}

@Composable
private fun RowScope.ScheduleCells(channel: TvChannel, focus: GuideFocus, start: Long, duration: Long, now: Long, active: Boolean, select: (Long) -> Unit) {
    val end = start + duration
    val cells = remember(channel.programmes, start, duration) {
        val result = mutableListOf<Triple<Long, Long, TvProgramme?>>()
        var cursor = start
        for (p in channel.programmes.filter { it.start < end && it.end > start }.sortedWith(compareBy({ it.start }, { it.id }))) {
            if (p.end <= cursor) continue
            if (p.start > cursor) result.add(Triple(cursor, minOf(p.start, end), null))
            val left = maxOf(cursor, p.start); val right = minOf(p.end, end)
            if (right > left) result.add(Triple(left, right, p))
            cursor = maxOf(cursor, right)
        }
        if (cursor < end) result.add(Triple(cursor, end, null))
        result
    }
    BoxWithConstraints(Modifier.weight(1f).fillMaxHeight()) {
        Row(Modifier.fillMaxSize()) {
            for ((left, right, p) in cells) {
                val selected = active && focus.time >= left && focus.time < right
                Box(Modifier.weight((right - left).toFloat()).fillMaxHeight().background(if (selected) Color(0xff355869) else Panel)
                    .border(1.dp, Color(0xff30414d)).padding(7.dp).semantics {
                        contentDescription = "${channel.name}, ${p?.title ?: "No programme information"}, ${timeLabel(left)} to ${timeLabel(right)}"
                        this.selected = selected; onClick("Select programme") { select(maxOf(left, minOf(focus.time, right - 1))); true }
                    }, contentAlignment = Alignment.CenterStart) {
                    Text(p?.title ?: "No programme information", fontSize = 16.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
            }
        }
        if (now in start..end) Box(Modifier.offset(x = maxWidth * ((now - start).toFloat() / duration)).width(2.dp).fillMaxHeight().background(Aqua).semantics { contentDescription = "Current time" })
    }
}

@Composable
internal fun PanelPage(title: String, content: @Composable ColumnScope.() -> Unit) {
    Column(Modifier.fillMaxSize().background(Canvas.copy(alpha = .97f)).padding(32.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Text(title, color = Aqua, fontSize = 28.sp, fontWeight = FontWeight.Bold)
        content()
    }
}

@UnstableApi
@Composable
private fun QuickControls(vm: TvViewModel) = PanelPage("Live TV") {
    val choices = listOf("Guide" to TvSurface.GUIDE, "Favorites" to TvSurface.FAVORITES, "Recents" to TvSurface.RECENTS,
        "Program information" to TvSurface.DETAILS, "Audio" to TvSurface.AUDIO, "Captions" to TvSurface.CAPTIONS,
        "Playback" to TvSurface.PLAYBACK, "Settings" to TvSurface.SETTINGS, "Search" to TvSurface.SEARCH, "Controls help" to TvSurface.HELP)
    LazyVerticalGrid(columns = GridCells.Fixed(3), horizontalArrangement = Arrangement.spacedBy(16.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        items(choices) { (label, screen) -> TvAction(label, vm, Modifier.height(66.dp), first = screen == TvSurface.GUIDE) { vm.show(screen) } }
        item { TvAction("Previous channel", vm) { vm.previousChannel() } }
        item { TvAction("Back to live TV", vm) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun FavoritesScreen(vm: TvViewModel) = PanelPage("Favorites") {
    val preferences by vm.repository.preferences.collectAsStateWithLifecycle()
    val settings by vm.settings.collectAsStateWithLifecycle()
    LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        if (preferences.favorites.isEmpty()) item { Text("Create a favorites list here or in the web viewer's Lists page.", color = Muted) }
        items(preferences.favorites, key = { it.id }) { list ->
            TvAction("${list.name} • ${list.channels.size} channels", vm, first = list == preferences.favorites.firstOrNull()) {
                vm.updateSettings(settings.copy(favoriteList = list.id, groupId = 0, category = ""))
                vm.back(); vm.show(TvSurface.GUIDE)
            }
        }
        item { TvAction("Manage favorites and channel order", vm, first = preferences.favorites.isEmpty()) { vm.show(TvSurface.CHANNELS) } }
        item { TvAction("Back", vm) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun FilterScreen(vm: TvViewModel) = PanelPage("Guide filters") {
    val s by vm.settings.collectAsStateWithLifecycle(); val channels by vm.repository.channels.collectAsStateWithLifecycle()
    val lineups by vm.repository.lineups.collectAsStateWithLifecycle(); val preferences by vm.repository.preferences.collectAsStateWithLifecycle()
    val categoryNames by vm.repository.categories.collectAsStateWithLifecycle()
    LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item { TvAction("All accessible lineups${if (s.lineupId == 0L) " ✓" else ""}", vm, first = true) { vm.updateSettings(s.copy(lineupId = 0, groupId = 0)); vm.back() } }
        items(lineups, key = { "lineup:${it.id}" }) { lineup -> TvAction("Lineup: ${lineup.name}${if (s.lineupId == lineup.id) " ✓" else ""}", vm) { vm.updateSettings(s.copy(lineupId = lineup.id, groupId = 0)); vm.back() } }
        item { TvAction("All groups", vm) { vm.updateSettings(s.copy(groupId = 0)); vm.back() } }
        items(channels.filter { s.lineupId == 0L || it.lineupId == s.lineupId }.distinctBy { it.groupId }, key = { "group:${it.groupId}" }) { c -> TvAction("Group: ${c.groupName}", vm) { vm.updateSettings(s.copy(groupId = c.groupId)); vm.back() } }
        item { TvAction("All channels (no favorites filter)", vm) { vm.updateSettings(s.copy(favoriteList = "")); vm.back() } }
        items(preferences.favorites, key = { "favorite:${it.id}" }) { list -> TvAction("Favorites: ${list.name}", vm) { vm.updateSettings(s.copy(favoriteList = list.id)); vm.back() } }
        item { TvAction("All programme categories", vm) { vm.updateSettings(s.copy(category = "")); vm.back() } }
        items((categoryNames.filterKeys { s.lineupId == 0L || it == s.lineupId }.values.flatten() + channels.filter { s.lineupId == 0L || it.lineupId == s.lineupId }.flatMap { it.at(System.currentTimeMillis())?.categories.orEmpty() }).distinctBy { it.lowercase() }.sorted(), key = { "category:$it" }) { category -> TvAction("On now: $category", vm) { vm.updateSettings(s.copy(category = category)); vm.back() } }
        item { TvAction("Close filters", vm) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun DateScreen(vm: TvViewModel) = PanelPage("Choose guide date") {
    val focus by vm.focus.collectAsStateWithLifecycle()
    LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        items((-1L..7L).toList()) { offset ->
            val date = java.time.LocalDate.now().plusDays(offset)
            TvAction(date.format(DateTimeFormatter.ofPattern("EEEE, MMMM d")), vm, first = offset == 0L) {
                val current = Instant.ofEpochMilli(focus.time).atZone(ZoneId.systemDefault()).toLocalDate()
                vm.day(java.time.temporal.ChronoUnit.DAYS.between(current, date)); vm.back()
            }
        }
        item { TvAction("Back", vm) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun DetailsScreen(vm: TvViewModel) {
    val channels by vm.repository.channels.collectAsStateWithLifecycle(); val focus by vm.focus.collectAsStateWithLifecycle()
    val status by vm.playback.status.collectAsStateWithLifecycle()
    val channel = channels.find { it.key == focus.channelKey } ?: status.channel
    val programme = channel?.at(focus.time)
    PanelPage(channel?.name ?: "Programme information") {
        Column(Modifier.weight(1f).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(programme?.title ?: "No programme information", fontSize = 30.sp)
            programme?.let { Text("${dateLabel(it.start)}  ${timeLabel(it.start)} – ${timeLabel(it.end)}", color = Muted); Text(it.subtitle); Text(it.description, fontSize = 20.sp); Text(it.categories.joinToString(" • "), color = Aqua) }
            Text("Live playback only. Past and future listings do not offer recording or catch-up.", color = Muted, fontSize = 16.sp)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
            if (channel != null) {
                TvAction("Watch channel live", vm, first = true) { vm.tune(channel) }
                TvAction("Toggle favorite", vm) { vm.favorite(channel) }
            }
            TvAction("Back", vm, first = channel == null) { vm.back() }
        }
    }
}

@UnstableApi
@Composable
private fun RecentScreen(vm: TvViewModel) = PanelPage("Recently watched on this TV") {
    val recents by vm.recents.collectAsStateWithLifecycle(); val channels by vm.repository.channels.collectAsStateWithLifecycle()
    LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        items(recents, key = { it }) { key ->
            val channel = channels.find { it.key == key }
            TvAction(channel?.let { "${it.number}  ${it.name}" } ?: "Load channel $key", vm, first = key == recents.firstOrNull()) { vm.task { vm.repository.channel(key)?.let(vm::tune) } }
        }
        item { TvAction("Back", vm, first = recents.isEmpty()) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun SearchScreen(vm: TvViewModel) = PanelPage("Search channels and programmes") {
    var term by remember { mutableStateOf("") }; var all by remember { mutableStateOf(false) }
    val results by vm.searchResults.collectAsStateWithLifecycle(); val cursor by vm.searchCursor.collectAsStateWithLifecycle()
    Row(horizontalArrangement = Arrangement.spacedBy(16.dp), verticalAlignment = Alignment.Bottom) {
        Field("Search", term, { term = it }, Modifier.weight(1f))
        TvAction(if (all) "All lineups" else "Current lineup", vm) { all = !all }
        TvAction("Search", vm, first = true) { vm.search(term, all) }
    }
    LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        items(results) { row -> TvAction("${if (row.optString("kind") == "channel") "Channel" else "Programme"}: ${row.optString("title")}", vm, Modifier.fillMaxWidth()) { vm.searchSelect(row) } }
        if (cursor != null) item { TvAction("Load more results", vm) { vm.search(term, all, true) } }
        item { TvAction("Back", vm) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun ChannelEditor(vm: TvViewModel) = PanelPage("Favorites and channel order") {
    val channels by vm.repository.channels.collectAsStateWithLifecycle(); val p by vm.repository.preferences.collectAsStateWithLifecycle()
    var name by remember { mutableStateOf("") }; var listIndex by remember { mutableIntStateOf(0) }
    val list = p.favorites.getOrNull(listIndex)
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Bottom) {
        Field("New favorites list", name, { name = it }, Modifier.weight(1f))
        TvAction("Create list", vm, first = true) { vm.createList(name); name = "" }
        TvAction("List: ${list?.name ?: "Favorites"}", vm) { listIndex = (listIndex + 1) % p.favorites.size.coerceAtLeast(1) }
    }
    Text("Changes sync to your account. For large edits, open Watch preferences in the web viewer.", color = Muted, fontSize = 15.sp)
    LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        val rank = p.order.withIndex().associate { it.value to it.index }
        items(channels.sortedWith(compareBy<TvChannel> { rank[it.key] ?: Int.MAX_VALUE }.thenBy { it.number }), key = { it.key }) { c ->
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("${c.number}  ${c.name}", Modifier.weight(1f), fontSize = 17.sp, maxLines = 2)
                TvAction(if (list?.channels?.contains(c.key) == true) "★ Remove" else "☆ Add", vm) { vm.favorite(c, list?.id) }
                TvAction("Up", vm) { vm.reorder(c, -1) }; TvAction("Down", vm) { vm.reorder(c, 1) }
                TvAction(if (c.key in p.hidden) "Unhide" else "Hide", vm) { vm.hide(c) }
            }
        }
        if (list != null) item { TvAction("Delete list ${list.name}", vm) { vm.deleteList(list.id); listIndex = 0 } }
        item { TvAction("Restore lineup order and show hidden channels", vm) { vm.restoreOrder() } }
        item { TvAction("Back", vm) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun TrackScreen(vm: TvViewModel, captions: Boolean) = PanelPage(if (captions) "Delivered caption tracks" else "Delivered audio tracks") {
    val status by vm.playback.status.collectAsStateWithLifecycle()
    val settings by vm.settings.collectAsStateWithLifecycle()
    val type = if (captions) C.TRACK_TYPE_TEXT else C.TRACK_TYPE_AUDIO
    val groups = status.tracks.groups.filter { it.type == type }
    LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        if (captions) item { TvAction("Captions off", vm, first = true) { vm.updateSettings(settings.copy(captions = false)) } }
        if (groups.isEmpty()) item { Text("This stream does not currently deliver ${if (captions) "caption" else "audio"} tracks.", color = Muted) }
        for (group in groups) for (i in 0 until group.length) if (group.isTrackSupported(i)) {
            item {
                val format = group.getTrackFormat(i)
                TvAction("${format.label ?: format.language ?: "Track ${i + 1}"}${if (group.isTrackSelected(i)) " ✓" else ""}", vm, first = !captions && group == groups.firstOrNull() && i == 0) {
                    vm.updateSettings(if (captions) settings.copy(captions = true, captionLanguage = format.language.orEmpty()) else settings.copy(audioLanguage = format.language.orEmpty()))
                    vm.playback.selectTrack(group, i)
                }
            }
        }
        item { TvAction("Save audio/caption preferences for this channel", vm) { vm.saveChannelOverride() } }
        item { TvAction("Use global preferences for this channel", vm) { vm.clearChannelOverride() } }
        item { TvAction("Back", vm, first = !captions && groups.isEmpty()) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun SettingsScreen(vm: TvViewModel, playbackOnly: Boolean) = PanelPage(if (playbackOnly) "Playback" else "TV settings") {
    val s by vm.settings.collectAsStateWithLifecycle(); val status by vm.playback.status.collectAsStateWithLifecycle()
    var advanced by remember { mutableStateOf(playbackOnly) }
    fun yes(value: Boolean) = if (value) "On" else "Off"
    fun next(value: String, options: List<String>) = options[(options.indexOf(value) + 1).mod(options.size)]
    LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        if (!playbackOnly) {
            item { TvAction("Startup: ${s.startup}", vm, first = true) { vm.updateSettings(s.copy(startup = next(s.startup, listOf("last", "guide", "chosen")))) } }
            item { TvAction("Use current channel at startup: ${status.channel?.name ?: "Choose a channel first"}", vm) { status.channel?.let { vm.updateSettings(s.copy(startup = "chosen", chosenChannel = it.key)) } } }
            item { TvAction("Favorites, order and hidden channels", vm) { vm.show(TvSurface.CHANNELS) } }
            item { TvAction("Guide: ${s.density.name.lowercase()} (${s.density.rows} rows / ${s.density.minutes} min)", vm) { vm.updateSettings(s.copy(density = GuideDensity.entries[(s.density.ordinal + 1) % GuideDensity.entries.size])) } }
            item { TvAction("Text size: ${(s.textScale * 100).toInt()}% (plus system scaling)", vm) { vm.updateSettings(s.copy(textScale = if (s.textScale >= 1.4f) 1f else s.textScale + .1f)) } }
            item { TvAction("Programme details: ${yes(s.showDetails)}", vm) { vm.updateSettings(s.copy(showDetails = !s.showDetails)) } }
            item { TvAction("Banner: ${s.bannerSeconds} seconds", vm) { vm.updateSettings(s.copy(bannerSeconds = if (s.bannerSeconds == 6) 2 else s.bannerSeconds + 2)) } }
            item { TvAction("Mini-guide: ${if (s.miniOverlay) "overlay" else "reduced video"}", vm) { vm.updateSettings(s.copy(miniOverlay = !s.miniOverlay)) } }
            item { TvAction("Audio tracks", vm) { vm.show(TvSurface.AUDIO) } }
            item { TvAction("Caption tracks", vm) { vm.show(TvSurface.CAPTIONS) } }
            item { TvAction("High contrast: ${yes(s.highContrast)}", vm) { vm.updateSettings(s.copy(highContrast = !s.highContrast)) } }
            item { TvAction("Reduced motion: ${yes(s.reducedMotion)}", vm) { vm.updateSettings(s.copy(reducedMotion = !s.reducedMotion)) } }
            item { TvAction("Controls help", vm) { vm.show(TvSurface.HELP) } }
            item { TvAction("About, license & source", vm) { vm.show(TvSurface.ABOUT) } }
        }
        item { TvAction("${if (advanced) "Hide" else "Show"} advanced settings", vm, first = playbackOnly) { advanced = !advanced } }
        if (advanced) {
            item { TvAction("Buffering: ${s.buffer.name.lowercase()}", vm) { vm.updateSettings(s.copy(buffer = BufferPreset.entries[(s.buffer.ordinal + 1) % BufferPreset.entries.size])) } }
            item { Text("Fast/Balanced/Stable start after 0.5/1/2 seconds buffered; recovery thresholds are twice that. Provider startup and keyframes still affect tuning.", color = Muted, fontSize = 15.sp) }
            item { TvAction("Frame-rate matching (seamless only): ${yes(s.frameRateMatching)}", vm) { vm.updateSettings(s.copy(frameRateMatching = !s.frameRateMatching)) } }
            item { TvAction("Pause / Resume live", vm) { vm.playback.pauseOrResume() } }
            item { TvAction("Return to live edge", vm) { vm.playback.player.seekToDefaultPosition(); vm.playback.player.play() } }
            item { TvAction("Aspect ratio: ${s.aspect}", vm) { vm.updateSettings(s.copy(aspect = next(s.aspect, listOf("fit", "zoom", "stretch")))) } }
            item { TvAction("Caption size: ${(s.captionScale * 100).toInt()}%", vm) { vm.updateSettings(s.copy(captionScale = if (s.captionScale >= 2f) .75f else s.captionScale + .25f)) } }
            item { TvAction("Surf all channels in lineup: ${yes(s.allChannelsSurf)}", vm) { vm.updateSettings(s.copy(allChannelsSurf = !s.allChannelsSurf)) } }
            item { TvAction("Wrap channel list: ${yes(s.wrap)}", vm) { vm.updateSettings(s.copy(wrap = !s.wrap)) } }
            item { TvAction("Browse before tune: ${yes(s.browseBeforeTune)}", vm) { vm.updateSettings(s.copy(browseBeforeTune = !s.browseBeforeTune)) } }
            item { TvAction("Left shortcut: ${s.leftShortcut}", vm) { vm.updateSettings(s.copy(leftShortcut = next(s.leftShortcut, listOf("previous", "mini", "guide", "recents", "info")))) } }
            item { TvAction("Right shortcut: ${s.rightShortcut}", vm) { vm.updateSettings(s.copy(rightShortcut = next(s.rightShortcut, listOf("mini", "previous", "guide", "recents", "info")))) } }
            item { Text("Home, Back, power, volume and system voice retain their platform roles. Navigation sounds follow system settings.", color = Muted, fontSize = 15.sp) }
        }
        item { TvAction("Clear recent channels on this TV", vm) { vm.clearHistory() } }
        item { TvAction("Clear guide and artwork cache", vm) { vm.clearCache() } }
        item { TvAction("Reset display and remote controls", vm) { vm.resetDisplay() } }
        item { TvAction("Connection diagnostics", vm) { vm.message.value = "${vm.auth.server()} • ${vm.auth.state.value} • ${vm.repository.channels.value.size} channels • First frame ${status.firstFrameMs?.let { "$it ms" } ?: "not measured"}" } }
        item { TvAction("Retry server connection", vm) { vm.connect() } }
        item { TvAction("Sign out and revoke this TV", vm) { vm.logout() } }
        item { TvAction("Back", vm) { vm.back() } }
    }
}

@UnstableApi
@Composable
private fun ControlsHelp(vm: TvViewModel) = PanelPage("Your remote, explained") {
    Column(Modifier.weight(1f).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("While watching", fontSize = 22.sp, color = Aqua)
        Text("↑ / ↓  Previous / next channel\nHold a surfing key to browse the banner; release to tune.\n←  Previous successfully watched channel\n→  Mini-guide, without interrupting your channel\nOK  Quick controls, starting with Guide", fontSize = 20.sp)
        Text("In the guide", fontSize = 22.sp, color = Aqua)
        Text("↑ / ↓  Change channel row, keeping the same programme time\n← / →  Move through programmes\nOK  Watch a current programme, or show details\nBack  Close the current screen; at live TV, exit\nFrom the first row, ↑ reaches filters, Now, day jumps and search.", fontSize = 20.sp)
        Text("If your remote has them: Guide, Info, Last, Channel/Page and number buttons also work. Every essential action is available with the D-pad and OK. No recording or extended rewind is offered.", color = Muted, fontSize = 17.sp)
    }
    TvAction("Got it — hide this first-use hint", vm, first = true) { vm.dismissHints() }
}
