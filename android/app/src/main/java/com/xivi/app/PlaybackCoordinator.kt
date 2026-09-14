package com.xivi.app

import android.content.Context
import android.content.Intent
import org.json.JSONObject
import java.util.concurrent.CopyOnWriteArraySet

data class PlaybackItem(
    val channelId: Long,
    val lineupId: Long,
    val name: String,
    val programme: String?,
    val logoUrl: String?,
    val streamUrl: String
) {
    fun toJson(): JSONObject = JSONObject()
        .put("channelId", channelId)
        .put("lineupId", lineupId)
        .put("name", name)
        .put("programme", programme)
        .put("logoUrl", logoUrl)
        .put("streamUrl", streamUrl)

    companion object {
        fun fromJson(json: JSONObject) = PlaybackItem(
            json.getLong("channelId"),
            json.getLong("lineupId"),
            json.getString("name"),
            json.optString("programme").takeIf { it.isNotBlank() && it != "null" },
            json.optString("logoUrl").takeIf { it.isNotBlank() && it != "null" },
            json.getString("streamUrl")
        )
    }
}

data class PlaybackState(
    val active: Boolean = false,
    val channelId: Long? = null,
    val name: String? = null,
    val programme: String? = null,
    val playing: Boolean = false,
    val loading: Boolean = false,
    val error: String? = null,
    val lineupId: Long? = null
) {
    fun toJson(): JSONObject = JSONObject()
        .put("active", active)
        .put("channelId", channelId)
        .put("name", name)
        .put("programme", programme)
        .put("playing", playing)
        .put("loading", loading)
        .put("error", error)
        .put("lineupId", lineupId)
}

@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
object PlaybackCoordinator {
    const val ACTION_PLAY = "com.xivi.app.PLAY"
    const val ACTION_STOP = "com.xivi.app.STOP"
    const val ACTION_OPEN_PLAYER = "com.xivi.app.OPEN_PLAYER"
    const val EXTRA_ITEM = "item"

    private val listeners = CopyOnWriteArraySet<(PlaybackState) -> Unit>()
    @Volatile private var state = PlaybackState()
    @Volatile private var currentItem: PlaybackItem? = null

    fun observe(listener: (PlaybackState) -> Unit): () -> Unit {
        listeners.add(listener)
        listener(state)
        return { listeners.remove(listener) }
    }

    fun state(): PlaybackState = state
    fun item(): PlaybackItem? = currentItem

    fun play(context: Context, item: PlaybackItem, openPlayer: Boolean) {
        currentItem = item
        context.getSharedPreferences("xivi_playback", Context.MODE_PRIVATE).edit()
            .putString("item", item.toJson().toString()).apply()
        val intent = Intent(context, XiviPlaybackService::class.java)
            .setAction(ACTION_PLAY)
            .putExtra(EXTRA_ITEM, item.toJson().toString())
        update(PlaybackState(true, item.channelId, item.name, item.programme, loading = true, lineupId = item.lineupId))
        context.startService(intent)
        if (openPlayer) {
            context.startActivity(playerIntent(context))
        }
    }

    fun restore(context: Context): PlaybackItem? {
        currentItem?.let { return it }
        val raw = context.getSharedPreferences("xivi_playback", Context.MODE_PRIVATE).getString("item", null) ?: return null
        return runCatching { PlaybackItem.fromJson(JSONObject(raw)) }.getOrNull().also { currentItem = it }
    }

    fun reopen(context: Context) {
        val item = restore(context) ?: throw IllegalStateException("Choose a channel to start watching.")
        if (state.active && state.error == null) {
            context.startActivity(playerIntent(context))
        } else {
            play(context, item, openPlayer = true)
        }
    }

    fun playerIntent(context: Context): Intent = Intent(context, MainActivity::class.java)
        .setAction(ACTION_OPEN_PLAYER)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)

    fun stop(context: Context) {
        context.startService(Intent(context, XiviPlaybackService::class.java).setAction(ACTION_STOP))
        currentItem = null
        context.getSharedPreferences("xivi_playback", Context.MODE_PRIVATE).edit().remove("item").apply()
        update(PlaybackState())
    }

    fun update(next: PlaybackState) {
        state = next
        listeners.forEach { it(next) }
    }
}
