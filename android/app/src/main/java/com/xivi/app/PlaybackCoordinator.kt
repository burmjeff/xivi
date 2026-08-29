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
    val streamUrl: String,
    val audioStreamUrl: String?
) {
    fun toJson(): JSONObject = JSONObject()
        .put("channelId", channelId)
        .put("lineupId", lineupId)
        .put("name", name)
        .put("programme", programme)
        .put("logoUrl", logoUrl)
        .put("streamUrl", streamUrl)
        .put("audioStreamUrl", audioStreamUrl)

    companion object {
        fun fromJson(json: JSONObject) = PlaybackItem(
            json.getLong("channelId"),
            json.getLong("lineupId"),
            json.getString("name"),
            json.optString("programme").takeIf { it.isNotBlank() && it != "null" },
            json.optString("logoUrl").takeIf { it.isNotBlank() && it != "null" },
            json.getString("streamUrl"),
            json.optString("audioStreamUrl").takeIf { it.isNotBlank() && it != "null" }
        )
    }
}

data class PlaybackState(
    val active: Boolean = false,
    val channelId: Long? = null,
    val name: String? = null,
    val programme: String? = null,
    val playing: Boolean = false,
    val audioOnly: Boolean = false
) {
    fun toJson(): JSONObject = JSONObject()
        .put("active", active)
        .put("channelId", channelId)
        .put("name", name)
        .put("programme", programme)
        .put("playing", playing)
        .put("audioOnly", audioOnly)
}

object PlaybackCoordinator {
    const val ACTION_PLAY = "com.xivi.app.PLAY"
    const val ACTION_STOP = "com.xivi.app.STOP"
    const val ACTION_CAR_CONNECTED = "com.xivi.app.CAR_CONNECTED"
    const val EXTRA_ITEM = "item"
    const val EXTRA_AUDIO_ONLY = "audio_only"

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

    fun play(context: Context, item: PlaybackItem, audioOnly: Boolean, openPlayer: Boolean) {
        currentItem = item
        context.getSharedPreferences("xivi_playback", Context.MODE_PRIVATE).edit()
            .putString("item", item.toJson().toString()).apply()
        val intent = Intent(context, XiviMediaLibraryService::class.java)
            .setAction(ACTION_PLAY)
            .putExtra(EXTRA_ITEM, item.toJson().toString())
            .putExtra(EXTRA_AUDIO_ONLY, audioOnly)
        context.startService(intent)
        update(PlaybackState(true, item.channelId, item.name, item.programme, true, audioOnly))
        if (openPlayer && !audioOnly) {
            context.startActivity(Intent(context, PlayerActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP))
        }
    }

    fun restore(context: Context): PlaybackItem? {
        currentItem?.let { return it }
        val raw = context.getSharedPreferences("xivi_playback", Context.MODE_PRIVATE).getString("item", null) ?: return null
        return runCatching { PlaybackItem.fromJson(JSONObject(raw)) }.getOrNull().also { currentItem = it }
    }

    fun reopen(context: Context) {
        if (state.active && !state.audioOnly) {
            context.startActivity(Intent(context, PlayerActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP))
        }
    }

    fun stop(context: Context) {
        context.startService(Intent(context, XiviMediaLibraryService::class.java).setAction(ACTION_STOP))
        currentItem = null
        context.getSharedPreferences("xivi_playback", Context.MODE_PRIVATE).edit().remove("item").apply()
        update(PlaybackState())
    }

    fun switchVideoToCarAudio(context: Context) {
        val item = currentItem ?: restore(context) ?: return
        if (!state.active || state.audioOnly || item.audioStreamUrl.isNullOrBlank()) return
        play(context, item, audioOnly = true, openPlayer = false)
        context.sendBroadcast(Intent(ACTION_CAR_CONNECTED).setPackage(context.packageName))
    }

    fun update(next: PlaybackState) {
        state = next
        listeners.forEach { it(next) }
    }
}

interface ParkedVideoCapability {
    val enabled: Boolean
}

object DisabledParkedVideoCapability : ParkedVideoCapability {
    override val enabled: Boolean = false
}
