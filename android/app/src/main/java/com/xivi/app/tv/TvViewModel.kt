package com.xivi.app.tv

import android.app.Application
import android.view.KeyEvent
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.util.UnstableApi
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import org.json.JSONObject
import java.time.Instant
import java.time.ZoneId

@UnstableApi
class TvViewModel(app: Application, val auth: TvAuthRepository) : AndroidViewModel(app) {
    constructor(app: Application) : this(app, TvAuthRepository.get(app))
    val repository = TvRepository(app, auth)
    val playback = TvPlayer(app, auth)
    val surface = MutableStateFlow(TvSurface.LIVE)
    val settings = MutableStateFlow(TvSettings())
    val focus = MutableStateFlow(GuideFocus())
    val message = MutableStateFlow("")
    val pairing = MutableStateFlow<JSONObject?>(null)
    val busy = MutableStateFlow(false)
    val banner = MutableStateFlow<TvChannel?>(null)
    val numberInput = MutableStateFlow("")
    val recents = MutableStateFlow<List<String>>(emptyList())
    val searchResults = MutableStateFlow<List<JSONObject>>(emptyList())
    val searchCursor = MutableStateFlow<String?>(null)
    val clock = MutableStateFlow(System.currentTimeMillis())
    val menuFocus = mutableMapOf<TvSurface, String>()
    private val stack = mutableListOf<TvSurface>()
    private var settingsJob: Job? = null
    private var historyJob: Job? = null
    private var bootJob: Job? = null
    private var guideJob: Job? = null
    private var bannerJob: Job? = null
    private var numberJob: Job? = null
    private var pairJob: Job? = null
    private var catalogJob: Job? = null
    private var categoryJob: Job? = null
    private var lastCategory = ""
    private var cachedCatalog: List<TvChannel>? = null
    private var cachedPreferences: ViewerPreferences? = null
    private var cachedSettings: TvSettings? = null
    private var cachedCategory: Set<String>? = null
    private var cachedActive: List<TvChannel> = emptyList()
    private var cachedSurf: List<TvChannel> = emptyList()
    private var previous: TvChannel? = null
    private var watched: TvChannel? = null
    private var pendingSurf: TvChannel? = null
    private var foreground = false
    private var started = false
    private var lastNamespace = auth.cacheNamespace()
    private var query = ""
    private var queryLineup: Long? = null

    init {
        playback.onWatched = { channel ->
            if (watched?.key != channel.key) { previous = watched; watched = channel }
            viewModelScope.launch { repository.settings().watched(channel.key); repository.remember(channel) }
        }
        viewModelScope.launch {
            var ticks = 0
            while (isActive) {
                delay(30_000); clock.value = System.currentTimeMillis()
                if (foreground && auth.hasCredential() && auth.state.value != TvAuthState.REVOKED) {
                    if (auth.state.value == TvAuthState.DISCONNECTED) task {
                        withContext(Dispatchers.IO) { auth.renew(false) }
                        if (!repository.catalogComplete.value) repository.refreshCatalog()
                    }
                    loadGuide(); refreshCategory()
                    if (++ticks % 2 == 0) task {
                        repository.refreshPreferences()
                        if (channels().none { it.key == focus.value.channelKey }) focus.value = focus.value.copy(channelKey = channels().firstOrNull()?.key)
                    }
                }
            }
        }
        viewModelScope.launch { auth.state.collect { if (it == TvAuthState.REVOKED) playback.stop() } }
    }
    fun channels(surfing: Boolean = false): List<TvChannel> {
        val s = settings.value
        val catalog = repository.channels.value; val preferences = repository.preferences.value; val category = repository.categoryKeys.value
        if (cachedCatalog !== catalog || cachedPreferences !== preferences || cachedSettings !== s || cachedCategory !== category) {
            val ordered = preferences.arrange(catalog).filter { s.lineupId == 0L || it.lineupId == s.lineupId }
            val favorites = preferences.favorites.find { it.id == s.favoriteList }?.channels?.toSet().orEmpty()
            cachedActive = ordered.filter { (s.groupId == 0L || it.groupId == s.groupId) && (s.favoriteList.isBlank() || it.key in favorites) && (s.category.isBlank() || category?.contains(it.key) == true) }
            cachedSurf = if (s.allChannelsSurf) ordered else cachedActive
            cachedCatalog = catalog; cachedPreferences = preferences; cachedSettings = s; cachedCategory = category
        }
        return if (surfing) cachedSurf else cachedActive
    }
    fun selected() = repository.channels.value.find { it.key == focus.value.channelKey } ?: banner.value ?: playback.status.value.channel
    fun onStart() { foreground = true; if (auth.hasCredential()) connect() }
    fun onStop() { foreground = false; pendingSurf = null; playback.stop() }
    fun connect() {
        bootJob?.cancel()
        bootJob = viewModelScope.launch {
            busy.value = true
            try {
                attachSettings(); repository.loadCached()
                runCatching { withContext(Dispatchers.IO) { auth.renew(false) } }.onFailure { message.value = it.message ?: "Temporarily disconnected. Pairing is saved." }
                if (auth.state.value == TvAuthState.REVOKED) return@launch
                val s = settings.value
                val key = if (s.startup == "chosen") s.chosenChannel else repository.settings().lastChannel.first().orEmpty()
                if (foreground && s.startup != "guide" && key.isNotBlank()) runCatching { repository.channel(key) }.getOrNull()?.let(::tune)
                if (s.startup == "guide" || playback.status.value.channel == null) show(TvSurface.GUIDE)
                if (!started) { started = true; if (settings.value.hints) show(TvSurface.HELP) }
                catalogJob?.cancel()
                catalogJob = viewModelScope.launch {
                    try {
                        repository.refreshPreferences()
                        repository.refreshCatalog { page ->
                            viewModelScope.launch {
                                if (foreground && playback.status.value.channel == null && settings.value.startup != "guide" && key.isBlank()) page.firstOrNull()?.let(::tune)
                                if (focus.value.channelKey == null) page.firstOrNull()?.let { focus.value = focus.value.copy(channelKey = it.key); loadGuide() }
                            }
                        }
                        refreshCategory()
                        loadGuide()
                    } catch (e: CancellationException) { throw e } catch (e: Exception) { message.value = e.message ?: "Showing saved guide; server is temporarily unavailable." }
                }
            } catch (e: CancellationException) { throw e } catch (e: Exception) { message.value = e.message.orEmpty() }
            finally { busy.value = false }
        }
    }
    private suspend fun attachSettings() {
        settingsJob?.cancel(); historyJob?.cancel()
        if (lastNamespace != auth.cacheNamespace()) { repository.resetIdentity(); lastNamespace = auth.cacheNamespace(); focus.value = GuideFocus(); watched = null; previous = null }
        val store = repository.settings(); settings.value = store.settings.first(); recents.value = store.recents.first()
        settingsJob = viewModelScope.launch { store.settings.collect { settings.value = it; playback.configure(effectiveSettings()); loadGuide() } }
        historyJob = viewModelScope.launch { store.recents.collect { recents.value = it } }
    }
    fun configureServer(server: String) = task {
        cancelIdentityWork()
        playback.stop(); repository.clearCache()
        withContext(Dispatchers.IO) { auth.configure(server) }
        beginPairing()
    }
    fun beginPairing() {
        pairJob?.cancel()
        pairJob = viewModelScope.launch {
            try {
                busy.value = true; pairing.value = withContext(Dispatchers.IO) { auth.beginPairing() }; busy.value = false
                val code = pairing.value!!.getString("device_code"); val deadline = System.currentTimeMillis() + 600_000
                while (isActive && System.currentTimeMillis() < deadline) {
                    delay(5_000)
                    try { if (withContext(Dispatchers.IO) { auth.poll(code) }) { pairing.value = null; connect(); return@launch } }
                    catch (e: TvApiException) { if (e.code == "invalid_grant") throw e; message.value = e.message.orEmpty() }
                    catch (e: java.io.IOException) { message.value = "Temporarily disconnected. Retrying pairing…" }
                }
                message.value = "Pairing code expired. Request a new code."
            } catch (e: CancellationException) { throw e } catch (e: Exception) { message.value = e.message.orEmpty() }
            finally { busy.value = false }
        }
    }
    fun tune(channel: TvChannel) {
        if (!foreground) return
        playback.configure(effectiveSettings(channel)); playback.tune(channel)
        focus.value = GuideFocus(channel.key, System.currentTimeMillis()); pendingSurf = null
        stack.clear(); surface.value = TvSurface.LIVE; displayBanner(channel); loadGuide()
    }
    fun previousChannel() {
        previous?.let(::tune) ?: task {
            val current = playback.status.value.channel?.key
            recents.value.firstOrNull { it != current }?.let { repository.channel(it) }?.let(::tune)
        }
    }
    fun show(next: TvSurface) {
        if (surface.value == next) return
        if (surface.value != TvSurface.QUICK) stack.add(surface.value)
        surface.value = next
        if (next == TvSurface.GUIDE || next == TvSurface.MINI) {
            if (focus.value.channelKey == null) focus.value = focus.value.copy(channelKey = playback.status.value.channel?.key ?: channels().firstOrNull()?.key)
            loadGuide()
        }
        if (next == TvSurface.FILTERS) task { repository.refreshCategories() }
    }
    fun back(): Boolean {
        if (numberInput.value.isNotEmpty()) { numberInput.value = ""; numberJob?.cancel(); return true }
        if (surface.value == TvSurface.LIVE) return false
        surface.value = if (stack.isNotEmpty()) stack.removeAt(stack.lastIndex) else TvSurface.LIVE
        return true
    }
    fun updateSettings(value: TvSettings) {
        settings.value = value
        if (channels().none { it.key == focus.value.channelKey }) focus.value = focus.value.copy(channelKey = channels().firstOrNull()?.key)
        if (value.category != lastCategory) { lastCategory = value.category; repository.categoryKeys.value = null; refreshCategory() }
        task { repository.settings().save(value); loadGuide() }
    }
    private fun refreshCategory() {
        categoryJob?.cancel()
        categoryJob = viewModelScope.launch {
            try { repository.category(settings.value.category); if (focus.value.channelKey !in channels().map { it.key }) focus.value = focus.value.copy(channelKey = channels().firstOrNull()?.key); loadGuide() }
            catch (e: CancellationException) { throw e } catch (e: Exception) { message.value = "Category filter could not be refreshed. ${e.message.orEmpty()}" }
        }
    }
    fun effectiveSettings(channel: TvChannel? = playback.status.value.channel): TvSettings {
        val s = settings.value
        val override = channel?.key?.let { s.channelOverrides[it] }?.let { runCatching { JSONObject(it) }.getOrNull() } ?: return s
        return s.copy(audioLanguage = override.optString("audioLanguage", s.audioLanguage), captionLanguage = override.optString("captionLanguage", s.captionLanguage),
            captions = override.optBoolean("captions", s.captions), aspect = override.optString("aspect", s.aspect))
    }
    fun saveChannelOverride() { playback.status.value.channel?.let { updateSettings(settings.value.copy(channelOverrides = settings.value.channelOverrides + (it.key to settings.value.json().toString()))) } }
    fun clearChannelOverride() { playback.status.value.channel?.let { updateSettings(settings.value.copy(channelOverrides = settings.value.channelOverrides - it.key)) } }
    fun displayBanner(channel: TvChannel) {
        banner.value = channel; bannerJob?.cancel()
        bannerJob = viewModelScope.launch { delay(settings.value.bannerSeconds * 1000L); if (pendingSurf == null) banner.value = null }
    }
    fun moveGuide(delta: Int): Boolean {
        val list = channels(); if (list.isEmpty()) return false
        val index = list.indexOfFirst { it.key == focus.value.channelKey }.coerceAtLeast(0)
        if (delta < 0 && index == 0) return false // D-pad can reach toolbar, without a dedicated Guide key.
        focus.value = focus.value.moveChannel(list, delta); loadGuide(); return true
    }
    fun moveTime(direction: Int) {
        focus.value = focus.value.moveTime(selected(), direction).let { it.copy(time = it.time.coerceIn(clock.value - 24 * 60 * 60_000L, clock.value + 7 * 24 * 60 * 60_000L)) }; loadGuide()
    }
    fun now() { focus.value = focus.value.copy(time = System.currentTimeMillis()); loadGuide() }
    fun day(delta: Long) {
        val next = Instant.ofEpochMilli(focus.value.time).atZone(ZoneId.systemDefault()).plusDays(delta).toInstant().toEpochMilli()
        focus.value = focus.value.copy(time = next.coerceIn(clock.value - 24 * 60 * 60_000L, clock.value + 7 * 24 * 60 * 60_000L)); loadGuide()
    }
    fun select() {
        val channel = selected() ?: return
        val program = channel.at(focus.value.time)
        if (program?.contains(System.currentTimeMillis()) == true || (program == null && kotlin.math.abs(focus.value.time - System.currentTimeMillis()) < 30 * 60_000)) tune(channel)
        else show(TvSurface.DETAILS)
    }
    fun loadGuide() {
        guideJob?.cancel()
        guideJob = viewModelScope.launch {
            delay(180)
            val list = channels(); val index = list.indexOfFirst { it.key == focus.value.channelKey }.coerceAtLeast(0)
            val visible = list.drop((index - 2).coerceAtLeast(0)).take(settings.value.density.rows + 4)
            try { repository.guide(visible, focus.value.time, settings.value.density.minutes) }
            catch (e: CancellationException) { throw e } catch (e: Exception) { message.value = "Saved listings • ${e.message ?: "connection unavailable"}" }
        }
    }
    fun favorite(channel: TvChannel, listID: String? = null) = task {
        val p = repository.preferences.value
        val id = listID ?: p.favorites.firstOrNull()?.id ?: "favorites"
        val list = p.favorites.find { it.id == id } ?: FavoriteList(id, "Favorites", emptyList())
        val changed = list.copy(channels = if (channel.key in list.channels) list.channels - channel.key else list.channels + channel.key)
        repository.savePreferences(p.copy(favorites = if (p.favorites.any { it.id == id }) p.favorites.map { if (it.id == id) changed else it } else p.favorites + changed))
    }
    fun createList(name: String) = task {
        require(name.isNotBlank()) { "Enter a list name" }
        val p = repository.preferences.value
        repository.savePreferences(p.copy(favorites = p.favorites + FavoriteList(java.util.UUID.randomUUID().toString(), name.trim(), emptyList())))
    }
    fun deleteList(id: String) = task { repository.savePreferences(repository.preferences.value.let { it.copy(favorites = it.favorites.filter { list -> list.id != id }) }) }
    fun hide(channel: TvChannel) = task { repository.savePreferences(repository.preferences.value.let { it.copy(hidden = if (channel.key in it.hidden) it.hidden - channel.key else it.hidden + channel.key) }) }
    fun reorder(channel: TvChannel, delta: Int) = task {
        val p = repository.preferences.value
        val keys = (p.order + repository.channels.value.map { it.key }).distinct().toMutableList()
        val from = keys.indexOf(channel.key); if (from >= 0) { val to = (from + delta).coerceIn(keys.indices); keys.removeAt(from); keys.add(to, channel.key); repository.savePreferences(p.copy(order = keys)) }
    }
    fun restoreOrder() = task { repository.savePreferences(repository.preferences.value.copy(order = emptyList(), hidden = emptySet())) }
    fun clearHistory() = task { repository.settings().clearHistory(); previous = null }
    fun clearCache() = task { repository.clearCache(); message.value = "Guide and artwork cache cleared. Pairing is unchanged." }
    fun resetDisplay() = updateSettings(settings.value.copy(density = GuideDensity.STANDARD, bannerSeconds = 4, showDetails = true, miniOverlay = true,
        highContrast = false, reducedMotion = false, textScale = 1f, captionScale = 1f, aspect = "fit", leftShortcut = "previous", rightShortcut = "mini"))
    private fun cancelIdentityWork() { bootJob?.cancel(); catalogJob?.cancel(); guideJob?.cancel(); categoryJob?.cancel(); settingsJob?.cancel(); historyJob?.cancel() }
    fun logout() = task { cancelIdentityWork(); playback.stop(); repository.clearCache(); withContext(Dispatchers.IO) { auth.logout() }; repository.resetIdentity(); pairing.value = null; started = false }
    fun dismissHints() { updateSettings(settings.value.copy(hints = false)); back() }
    fun search(term: String, allLineups: Boolean, more: Boolean = false) = task {
        query = term; queryLineup = if (allLineups) null else settings.value.lineupId.takeIf { it > 0 } ?: repository.lineups.value.firstOrNull()?.id
        var append = more
        val page = try { repository.search(term, queryLineup, if (more) searchCursor.value else null) }
        catch (e: TvApiException) { if (e.code == "invalid_cursor") { append = false; repository.search(term, queryLineup) } else throw e }
        searchResults.value = (if (append) searchResults.value else emptyList()) + page.optJSONArray("items").objects()
        searchCursor.value = page.optString("next_cursor").takeIf { it.isNotBlank() && it != "null" }
    }
    fun searchSelect(row: JSONObject) = task {
        val channel = repository.channel("${row.getLong("lineup_id")}:${row.getLong("channel_id")}") ?: return@task
        if (row.getString("kind") == "channel") tune(channel)
        else {
            val start = Instant.parse(row.getString("start")).toEpochMilli()
            repository.guide(listOf(channel), start, 120)
            focus.value = GuideFocus(channel.key, start); show(TvSurface.DETAILS)
        }
    }
    fun key(event: KeyEvent): Boolean {
        if (event.keyCode in listOf(KeyEvent.KEYCODE_HOME, KeyEvent.KEYCODE_VOLUME_UP, KeyEvent.KEYCODE_VOLUME_DOWN, KeyEvent.KEYCODE_VOLUME_MUTE, KeyEvent.KEYCODE_POWER, KeyEvent.KEYCODE_SEARCH, KeyEvent.KEYCODE_VOICE_ASSIST)) return false
        val active = surface.value
        if (auth.state.value !in listOf(TvAuthState.AUTHENTICATED, TvAuthState.DISCONNECTED, TvAuthState.CONNECTING)) return false
        val surf = event.keyCode in listOf(KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN, KeyEvent.KEYCODE_CHANNEL_UP, KeyEvent.KEYCODE_CHANNEL_DOWN, KeyEvent.KEYCODE_PAGE_UP, KeyEvent.KEYCODE_PAGE_DOWN)
        if (active == TvSurface.LIVE && surf) {
            if (event.action == KeyEvent.ACTION_DOWN) {
                val list = channels(true)
                val delta = if (event.keyCode in listOf(KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_CHANNEL_UP, KeyEvent.KEYCODE_PAGE_UP)) -1 else 1
                val selected = GuideFocus((pendingSurf ?: playback.status.value.channel)?.key).moveChannel(list, delta, settings.value.wrap)
                pendingSurf = list.find { it.key == selected.channelKey }; pendingSurf?.let(::displayBanner)
            } else if (event.action == KeyEvent.ACTION_UP) {
                if (!event.isCanceled) pendingSurf?.let { if (settings.value.browseBeforeTune) { focus.value = GuideFocus(it.key); show(TvSurface.MINI) } else tune(it) }
                pendingSurf = null
            }
            return true
        }
        if (active in listOf(TvSurface.GUIDE, TvSurface.MINI) && event.keyCode in listOf(KeyEvent.KEYCODE_PAGE_UP, KeyEvent.KEYCODE_PAGE_DOWN, KeyEvent.KEYCODE_CHANNEL_UP, KeyEvent.KEYCODE_CHANNEL_DOWN)) {
            if (event.action == KeyEvent.ACTION_DOWN) moveGuide(settings.value.density.rows * if (event.keyCode in listOf(KeyEvent.KEYCODE_PAGE_UP, KeyEvent.KEYCODE_CHANNEL_UP)) -1 else 1)
            return true
        }
        if (event.keyCode in KeyEvent.KEYCODE_0..KeyEvent.KEYCODE_9 && active in listOf(TvSurface.LIVE, TvSurface.GUIDE, TvSurface.MINI)) {
            if (event.action == KeyEvent.ACTION_DOWN && event.repeatCount == 0) {
                numberInput.value = (numberInput.value + (event.keyCode - KeyEvent.KEYCODE_0)).takeLast(5)
                numberJob?.cancel(); numberJob = viewModelScope.launch { delay(1400); commitNumber() }
            }; return true
        }
        if (event.action != KeyEvent.ACTION_DOWN || event.repeatCount > 0) return false
        when (event.keyCode) {
            KeyEvent.KEYCODE_GUIDE -> { if (active == TvSurface.GUIDE) show(TvSurface.FILTERS) else show(TvSurface.GUIDE); return true }
            KeyEvent.KEYCODE_INFO -> { show(TvSurface.DETAILS); return true }
            KeyEvent.KEYCODE_LAST_CHANNEL -> { previousChannel(); return true }
            KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE -> { playback.pauseOrResume(); return true }
            KeyEvent.KEYCODE_MEDIA_PLAY -> { playback.player.play(); return true }
            KeyEvent.KEYCODE_MEDIA_PAUSE -> { playback.player.pause(); return true }
            KeyEvent.KEYCODE_MEDIA_STOP -> { playback.stop(); return true }
            KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_ENTER -> {
                if (numberInput.value.isNotBlank()) { commitNumber(); return true }
                if (active == TvSurface.LIVE) { show(TvSurface.QUICK); return true }
            }
            KeyEvent.KEYCODE_DPAD_LEFT -> if (active == TvSurface.LIVE) { shortcut(settings.value.leftShortcut); return true }
            KeyEvent.KEYCODE_DPAD_RIGHT -> if (active == TvSurface.LIVE) { shortcut(settings.value.rightShortcut); return true }
        }
        return false
    }
    private fun shortcut(value: String) { when (value) { "previous" -> previousChannel(); "guide" -> show(TvSurface.GUIDE); "recents" -> show(TvSurface.RECENTS); "info" -> show(TvSurface.DETAILS); else -> show(TvSurface.MINI) } }
    private fun commitNumber() {
        numberJob?.cancel(); val number = numberInput.value.toLongOrNull(); numberInput.value = ""
        val channel = channels().find { it.number == number }
        if (channel == null) { message.value = "Channel $number is not available in the active list."; return }
        if (surface.value == TvSurface.LIVE) tune(channel) else { focus.value = focus.value.copy(channelKey = channel.key); loadGuide() }
    }
    fun task(action: suspend () -> Unit) { viewModelScope.launch { try { action() } catch (e: CancellationException) { throw e } catch (e: Exception) { message.value = e.message ?: "The operation could not be completed." } } }
    override fun onCleared() { playback.close(); super.onCleared() }
}
