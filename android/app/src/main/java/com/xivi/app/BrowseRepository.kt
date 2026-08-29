package com.xivi.app

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.net.URLEncoder
import java.security.MessageDigest

data class CarLineup(val id: Long, val name: String)
data class CarGroup(val id: Long, val lineupId: Long, val name: String)
data class CarChannel(
    val id: Long,
    val lineupId: Long,
    val groupId: Long,
    val name: String,
    val programme: String?,
    val programmeEnds: String?,
    val logoUrl: String?,
    val audioStreamUrl: String?
)

class BrowseRepository(
    private val context: Context,
    private val auth: AuthRepository = AuthRepository.get(context)
) {
    private val cache = context.getSharedPreferences("xivi_car_library", Context.MODE_PRIVATE)

    fun lineups(): List<CarLineup> = pageItems("/api/v2/watch/lineups")
        .map { CarLineup(it.getLong("id"), it.getString("name")) }

    fun groups(lineupId: Long): List<CarGroup> =
        pageItems("/api/v2/watch/lineups/$lineupId/groups")
            .map { CarGroup(it.getLong("id"), lineupId, it.getString("name")) }

    fun channels(lineupId: Long, groupId: Long? = null, query: String? = null): List<CarChannel> {
        val parameters = mutableListOf("limit=500")
        if (groupId != null) parameters += "group_id=$groupId"
        if (!query.isNullOrBlank()) parameters += "q=${URLEncoder.encode(query, Charsets.UTF_8.name())}"
        return pageItems("/api/v2/watch/lineups/$lineupId/channels?${parameters.joinToString("&")}")
            .mapNotNull { item ->
                val audio = item.opt("audio_stream_url")?.takeUnless { it == JSONObject.NULL }?.toString()
                if (audio.isNullOrBlank()) return@mapNotNull null
                val current = item.optJSONObject("current")
                CarChannel(
                    item.getLong("id"), lineupId, item.getLong("group_id"), item.getString("name"),
                    current?.optString("title")?.takeIf { it.isNotBlank() },
                    current?.optString("end")?.takeIf { it.isNotBlank() },
                    item.optString("logo_url").takeIf { it.isNotBlank() }, audio
                )
            }
    }

    fun search(query: String): List<CarChannel> = lineups().flatMap { channels(it.id, query = query) }
        .distinctBy { "${it.lineupId}:${it.id}" }

    private fun pageItems(firstPath: String): List<JSONObject> {
        val items = mutableListOf<JSONObject>()
        var path: String? = firstPath
        while (path != null) {
            val json = cachedRequest(path)
            val page = json.optJSONArray("items") ?: JSONArray()
            for (index in 0 until page.length()) items += page.getJSONObject(index)
            val cursor = json.opt("next_cursor")?.takeUnless { it == JSONObject.NULL }?.toString()
            path = if (cursor.isNullOrBlank()) null else {
                val separator = if (firstPath.contains('?')) '&' else '?'
                "$firstPath${separator}cursor=${URLEncoder.encode(cursor, Charsets.UTF_8.name())}"
            }
        }
        return items
    }

    private fun cachedRequest(path: String): JSONObject {
        val key = MessageDigest.getInstance("SHA-256").digest(path.toByteArray())
            .joinToString("") { "%02x".format(it) }
        return try {
            val response = auth.request(path, "GET", mapOf("Accept" to "application/json"))
            if (response.status == 401) throw SecurityException("Sign in on your phone")
            require(response.status in 200..299) { "The car library is temporarily unavailable" }
            cache.edit().putString(key, response.body).apply()
            JSONObject(response.body)
		} catch (error: Exception) {
			if (error is SecurityException) throw error
			val saved = cache.getString(key, null) ?: throw error
            JSONObject(saved)
        }
    }

    companion object {
        fun clear(context: Context) {
            context.getSharedPreferences("xivi_car_library", Context.MODE_PRIVATE).edit().clear().apply()
        }
    }
}
