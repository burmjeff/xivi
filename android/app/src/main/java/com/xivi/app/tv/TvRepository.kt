package com.xivi.app.tv

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.LruCache
import com.xivi.app.ArtworkCache
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import java.net.URLEncoder
import java.time.Instant

internal fun String.urlEncoded(): String = URLEncoder.encode(this, "UTF-8")

class TvRepository(private val context: Context, val auth: TvAuthRepository) {
    private val cache = TvCacheDatabase.get(context).entries()
    private val catalog = MutableStateFlow<List<TvChannel>>(emptyList())
    val channels = catalog.asStateFlow()
    val lineups = MutableStateFlow<List<TvLineup>>(emptyList())
    val preferences = MutableStateFlow(ViewerPreferences())
    val guideUpdatedAt = MutableStateFlow(0L)
    val catalogComplete = MutableStateFlow(false)
    val categoryKeys = MutableStateFlow<Set<String>?>(null)
    val categories = MutableStateFlow<Map<Long, List<String>>>(emptyMap())
    private val bitmaps = object : LruCache<String, Bitmap>(12 * 1024 * 1024) { override fun sizeOf(key: String, value: Bitmap) = value.byteCount }
    fun settings() = TvSettingsStore(context, auth.cacheNamespace())
    fun resetIdentity() {
        catalog.value = emptyList(); lineups.value = emptyList(); preferences.value = ViewerPreferences()
        categoryKeys.value = null; categories.value = emptyMap(); guideUpdatedAt.value = 0; catalogComplete.value = false; bitmaps.evictAll()
    }
    private suspend fun fetch(path: String, method: String = "GET", body: JSONObject? = null): JSONObject {
        val namespace = auth.cacheNamespace()
        val result = auth.json(path, method, body)
        currentCoroutineContext().ensureActive()
        if (namespace != auth.cacheNamespace()) throw CancellationException("TV identity changed")
        return result
    }

    private suspend fun cached(key: String): JSONObject? = cache.get(auth.cacheNamespace(), key)?.let { runCatching { JSONObject(it.json) }.getOrNull() }
    private suspend fun save(key: String, json: JSONObject) {
        currentCoroutineContext().ensureActive()
        val raw = json.toString()
        if (raw.length <= 2 * 1024 * 1024) cache.put(TvCacheEntry(auth.cacheNamespace(), key, raw, System.currentTimeMillis()))
        cache.prune(System.currentTimeMillis() - 14 * 24 * 60 * 60_000L)
        while (cache.bytes() > 32 * 1024 * 1024) cache.evictOldest()
    }
    private fun merge(items: List<TvChannel>, replaceFrom: Long? = null, replaceTo: Long? = null) {
        catalog.update { existing ->
            val all = existing.associateBy { it.key }.toMutableMap()
            items.forEach { channel ->
                val previous = all[channel.key]
                val retained = previous?.programmes.orEmpty().filter { replaceFrom == null || replaceTo == null || it.end <= replaceFrom || it.start >= replaceTo }
                val programs = (retained + channel.programmes).associateBy { it.id }.values
                    .filter { it.end > System.currentTimeMillis() - 24 * 60 * 60_000L }.sortedBy { it.start }.take(500)
                all[channel.key] = channel.copy(programmes = programs)
            }
            all.values.toList()
        }
    }
    suspend fun loadCached() = withContext(Dispatchers.IO) {
        cached("lineups")?.let { lineups.value = it.optJSONArray("items").objects().map { j -> TvLineup(j.getLong("id"), j.getString("name")) } }
        cached("preferences")?.let { preferences.value = ViewerPreferences.parse(it) }
        for (lineup in lineups.value) cached("groups:${lineup.id}")?.let { groups ->
            categories.update { it + (lineup.id to groups.optJSONArray("on_now_categories").strings()) }
        }
        for (lineup in lineups.value) cached("catalog:${lineup.id}")?.let { merge(it.optJSONArray("items").objects().map { j -> TvChannel.parse(j, lineup.id) }) }
        for (entry in cache.recentGuides(auth.cacheNamespace()).asReversed()) {
            val lineup = entry.key.split(':').getOrNull(1)?.toLongOrNull() ?: continue
            runCatching { JSONObject(entry.json) }.getOrNull()?.let { merge(it.optJSONArray("items").objects().map { j -> TvChannel.parse(j, lineup) }) }
            guideUpdatedAt.value = entry.updatedAt
        }
    }
    suspend fun channel(key: String): TvChannel? = withContext(Dispatchers.IO) {
        catalog.value.find { it.key == key }?.let { return@withContext it }
        val parts = key.split(':'); if (parts.size != 2) return@withContext null
        cached("channel:$key")?.let { return@withContext TvChannel.parse(it, parts[0].toLong()) }
        val json = fetch("/api/v2/watch/channels/${parts[1]}?lineup_id=${parts[0]}")
        save("channel:$key", json); TvChannel.parse(json, parts[0].toLong()).also { merge(listOf(it)) }
    }
    suspend fun remember(channel: TvChannel) = withContext(Dispatchers.IO) { save("channel:${channel.key}", channel.json()) }
    suspend fun refreshCatalog(onFirstPage: (List<TvChannel>) -> Unit = {}) = withContext(Dispatchers.IO) {
        val json = fetch("/api/v2/watch/lineups"); save("lineups", json)
        lineups.value = json.optJSONArray("items").objects().map { TvLineup(it.getLong("id"), it.getString("name")) }
        val granted = lineups.value.map { it.id }.toSet(); catalog.update { it.filter { ch -> ch.lineupId in granted } }
        for (lineup in lineups.value) {
            val collected = linkedMapOf<String, TvChannel>(); var cursor: String? = null; var restarted = false
            do {
                val path = "/api/v2/watch/lineups/${lineup.id}/channels?metadata_only=true&limit=250" + (cursor?.let { "&cursor=${it.urlEncoded()}" } ?: "")
                val page = try { fetch(path) } catch (e: TvApiException) {
                    if (e.code == "invalid_cursor" && !restarted) { cursor = null; restarted = true; continue } else throw e
                }
                val items = page.optJSONArray("items").objects().map { TvChannel.parse(it, lineup.id) }
                items.forEach { collected[it.key] = it }; merge(items); onFirstPage(items)
                cursor = page.optString("next_cursor").takeIf { it.isNotBlank() && it != "null" }
            } while (cursor != null)
            catalog.update { old -> old.filter { it.lineupId != lineup.id || it.key in collected } }
            save("catalog:${lineup.id}", JSONObject().put("items", JSONArray(collected.values.map { it.json() })))
        }
        catalogComplete.value = true
    }
    suspend fun guide(visible: List<TvChannel>, anchor: Long, minutes: Int) = withContext(Dispatchers.IO) {
        val start = Math.floorDiv(anchor, 30 * 60_000L) * 30 * 60_000L
        for ((lineup, rows) in visible.groupBy { it.lineupId }) {
            val ids = rows.take(30).map { it.id }.distinct().sorted().joinToString(",")
            if (ids.isEmpty()) continue
            val key = "guide:$lineup:$ids:$start:$minutes"
            val existing = cache.get(auth.cacheNamespace(), key)
            if (existing != null) {
                runCatching { JSONObject(existing.json) }.getOrNull()?.let { merge(it.optJSONArray("items").objects().map { j -> TvChannel.parse(j, lineup) }) }
                guideUpdatedAt.value = existing.updatedAt
            }
            val from = Instant.ofEpochMilli(start).toString().urlEncoded()
            val to = Instant.ofEpochMilli(start + (minutes + 60) * 60_000L).toString().urlEncoded()
            val json = fetch("/api/v2/watch/lineups/$lineup/guide?channel_ids=$ids&from=$from&to=$to&limit=100")
            merge(json.optJSONArray("items").objects().map { TvChannel.parse(it, lineup) }, start, start + (minutes + 60) * 60_000L); save(key, json)
            guideUpdatedAt.value = System.currentTimeMillis()
        }
    }
    suspend fun refreshPreferences() = withContext(Dispatchers.IO) {
        val json = fetch("/api/v2/watch/preferences"); preferences.value = ViewerPreferences.parse(json); save("preferences", json)
    }
    suspend fun refreshCategories() = withContext(Dispatchers.IO) {
        for (lineup in lineups.value) {
            val groups = fetch("/api/v2/watch/lineups/${lineup.id}/groups")
            categories.update { it + (lineup.id to groups.optJSONArray("on_now_categories").strings()) }
            save("groups:${lineup.id}", groups)
        }
    }
    suspend fun savePreferences(value: ViewerPreferences) = withContext(Dispatchers.IO) {
        try { val json = fetch("/api/v2/watch/preferences", "PATCH", value.json()); preferences.value = ViewerPreferences.parse(json); save("preferences", json) }
        catch (e: TvApiException) { if (e.status == 409) refreshPreferences(); throw e }
    }
    suspend fun search(term: String, lineup: Long?, cursor: String? = null): JSONObject = withContext(Dispatchers.IO) {
        fetch("/api/v2/watch/search?q=${term.urlEncoded()}&limit=50" + (lineup?.let { "&lineup_id=$it" } ?: "") + (cursor?.let { "&cursor=${it.urlEncoded()}" } ?: ""))
    }
    suspend fun category(category: String) = withContext(Dispatchers.IO) {
        if (category.isBlank()) { categoryKeys.value = null; return@withContext }
        val keys = mutableSetOf<String>()
        for (lineup in lineups.value) {
            var cursor: String? = null; var restarted = false
            do {
                val page = try { fetch("/api/v2/watch/lineups/${lineup.id}/channels?metadata_only=true&limit=250&on_now_category=${category.urlEncoded()}" + (cursor?.let { "&cursor=${it.urlEncoded()}" } ?: "")) }
                    catch (e: TvApiException) { if (e.code == "invalid_cursor" && !restarted) { cursor = null; restarted = true; continue }; throw e }
                val items = page.optJSONArray("items").objects().map { TvChannel.parse(it, lineup.id) }
                keys.addAll(items.map { it.key }); merge(items)
                cursor = page.optString("next_cursor").takeIf { it.isNotBlank() && it != "null" }
            } while (cursor != null)
        }
        categoryKeys.value = keys
    }
    suspend fun logo(channel: TvChannel): Bitmap? = withContext(Dispatchers.IO) {
        val key = auth.cacheNamespace() + channel.logo
        bitmaps.get(key)?.let { return@withContext it }
        val uri = ArtworkCache.contentUri(context, auth, channel.logo) ?: return@withContext null
        runCatching {
            val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
            context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, options) }
            options.inSampleSize = 1
            while (options.outWidth / options.inSampleSize > 512 || options.outHeight / options.inSampleSize > 512) options.inSampleSize *= 2
            options.inJustDecodeBounds = false
            context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, options) }
        }.getOrNull()?.also { bitmaps.put(key, it) }
    }
    suspend fun clearCache() = withContext(Dispatchers.IO) { cache.clear(auth.cacheNamespace()); bitmaps.evictAll(); ArtworkCache.clear(context) }
}

internal fun TvChannel.json() = JSONObject().put("id", id).put("number", number).put("name", name).put("group_id", groupId)
    .put("group_name", groupName).put("logo_url", logo).put("stream_url", streamUrl).put("programmes", JSONArray(programmes.map {
        JSONObject().put("id", it.id).put("title", it.title).put("start", Instant.ofEpochMilli(it.start)).put("end", Instant.ofEpochMilli(it.end))
            .put("subtitle", it.subtitle).put("description", it.description).put("categories", JSONArray(it.categories))
    }))
