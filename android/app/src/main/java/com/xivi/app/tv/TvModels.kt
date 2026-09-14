package com.xivi.app.tv

import org.json.JSONArray
import org.json.JSONObject
import java.time.Instant

data class TvProgramme(
    val id: Long, val title: String, val start: Long, val end: Long,
    val subtitle: String = "", val description: String = "", val categories: List<String> = emptyList()
) {
    fun contains(time: Long) = start <= time && time < end
    companion object {
        fun parse(json: JSONObject) = TvProgramme(json.optLong("id"), json.optString("title"),
            Instant.parse(json.getString("start")).toEpochMilli(), Instant.parse(json.getString("end")).toEpochMilli(),
            json.optString("subtitle"), json.optString("description"), json.optJSONArray("categories").strings())
    }
}

data class TvChannel(
    val id: Long, val lineupId: Long, val number: Long, val name: String,
    val groupId: Long, val groupName: String, val logo: String, val streamUrl: String,
    val programmes: List<TvProgramme> = emptyList()
) {
    val key get() = "$lineupId:$id"
    fun at(time: Long): TvProgramme? = programmes.filter { it.contains(time) }.minWithOrNull(compareBy({ it.start }, { it.id }))
    fun next(time: Long): TvProgramme? = programmes.filter { it.start > time }.minByOrNull { it.start }
    companion object {
        fun parse(json: JSONObject, lineupId: Long) = TvChannel(json.getLong("id"), lineupId,
            json.optLong("number"), json.getString("name"), json.optLong("group_id"),
            json.optString("group_name"), json.optString("logo_url"), json.getString("stream_url"),
            json.optJSONArray("programmes").objects().mapNotNull { runCatching { TvProgramme.parse(it) }.getOrNull() }
                .filter { it.end > it.start }.sortedWith(compareBy({ it.start }, { it.id })))
    }
}

data class TvLineup(val id: Long, val name: String)
enum class TvSurface { LIVE, QUICK, GUIDE, MINI, FILTERS, FAVORITES, DETAILS, RECENTS, SEARCH, SETTINGS, CHANNELS, HELP, AUDIO, CAPTIONS, PLAYBACK, DATE }
enum class GuideDensity(val rows: Int, val minutes: Int) { STANDARD(7, 120), COMPACT(10, 180), LARGE(5, 90) }
enum class BufferPreset(val startupMs: Int) { FAST(500), BALANCED(1000), STABLE(2000) }
enum class TvAuthState { UNCONFIGURED, UNPAIRED, CONNECTING, AUTHENTICATED, DISCONNECTED, REVOKED }

/** Focus is a channel identity plus an instant, never a programme index. */
data class GuideFocus(val channelKey: String? = null, val time: Long = System.currentTimeMillis()) {
    fun moveChannel(channels: List<TvChannel>, delta: Int, wrap: Boolean = false): GuideFocus {
        if (channels.isEmpty()) return this
        val index = channels.indexOfFirst { it.key == channelKey }.coerceAtLeast(0)
        val next = if (wrap) Math.floorMod(index + delta, channels.size) else (index + delta).coerceIn(channels.indices)
        return copy(channelKey = channels[next].key)
    }
    fun moveTime(channel: TvChannel?, direction: Int): GuideFocus {
        val current = channel?.at(time)
        val next = if (direction > 0) current?.end ?: (time + 30 * 60_000L)
            else current?.let { it.start - 1 } ?: (time - 30 * 60_000L)
        return copy(time = next)
    }
}

internal fun JSONArray?.objects(): List<JSONObject> = if (this == null) emptyList() else (0 until length()).mapNotNull { optJSONObject(it) }
internal fun JSONArray?.strings(): List<String> = if (this == null) emptyList() else (0 until length()).map { optString(it) }

data class FavoriteList(val id: String, val name: String, val channels: List<String>)
data class ViewerPreferences(val revision: Long = 0, val favorites: List<FavoriteList> = emptyList(),
    val hidden: Set<String> = emptySet(), val order: List<String> = emptyList()) {
    fun json() = JSONObject().put("revision", revision).put("favorites", JSONArray(favorites.map {
        JSONObject().put("id", it.id).put("name", it.name).put("channels", JSONArray(it.channels))
    })).put("hidden", JSONArray(hidden.toList())).put("order", JSONArray(order))
    fun favorite(key: String) = favorites.any { key in it.channels }
    fun arrange(channels: List<TvChannel>): List<TvChannel> {
        val ranks = order.withIndex().associate { it.value to it.index }
        return channels.filter { it.key !in hidden }.sortedWith(compareBy<TvChannel> { ranks[it.key] ?: Int.MAX_VALUE }.thenBy { it.number }.thenBy { it.id })
    }
    companion object {
        fun parse(json: JSONObject) = ViewerPreferences(json.optLong("revision"), json.optJSONArray("favorites").objects().map {
            FavoriteList(it.getString("id"), it.getString("name"), it.optJSONArray("channels").strings())
        }, json.optJSONArray("hidden").strings().toSet(), json.optJSONArray("order").strings())
    }
}
