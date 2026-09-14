package com.xivi.app.tv

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import androidx.room.*
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import org.json.JSONObject

@Entity(tableName = "tv_cache", primaryKeys = ["namespace", "key"])
data class TvCacheEntry(val namespace: String, val key: String, val json: String, val updatedAt: Long)

@Dao
interface TvCacheDao {
    @Query("SELECT * FROM tv_cache WHERE namespace = :namespace AND `key` = :key")
    suspend fun get(namespace: String, key: String): TvCacheEntry?
    @Query("SELECT * FROM tv_cache WHERE namespace = :namespace AND `key` LIKE 'guide:%' ORDER BY updatedAt DESC LIMIT 16")
    suspend fun recentGuides(namespace: String): List<TvCacheEntry>
    @Query("SELECT COALESCE(SUM(LENGTH(json)),0) FROM tv_cache") suspend fun bytes(): Long
    @Query("DELETE FROM tv_cache WHERE rowid IN (SELECT rowid FROM tv_cache ORDER BY updatedAt ASC LIMIT 16)") suspend fun evictOldest()
    @Insert(onConflict = OnConflictStrategy.REPLACE) suspend fun put(entry: TvCacheEntry)
    @Query("DELETE FROM tv_cache WHERE namespace = :namespace") suspend fun clear(namespace: String)
    @Query("DELETE FROM tv_cache WHERE updatedAt < :before OR rowid NOT IN (SELECT rowid FROM tv_cache ORDER BY updatedAt DESC LIMIT 256)")
    suspend fun prune(before: Long)
}

@Database(entities = [TvCacheEntry::class], version = 1, exportSchema = false)
abstract class TvCacheDatabase : RoomDatabase() {
    abstract fun entries(): TvCacheDao
    companion object {
        @Volatile private var instance: TvCacheDatabase? = null
        fun get(context: Context) = instance ?: synchronized(this) {
            instance ?: Room.databaseBuilder(context.applicationContext, TvCacheDatabase::class.java, "xivi-tv-cache.db")
                .build().also { instance = it }
        }
    }
}

private val Context.tvPreferences by preferencesDataStore("xivi_tv_preferences")

data class TvSettings(
	val lineupId: Long = 0, val groupId: Long = 0, val favoriteList: String = "", val category: String = "",
    val startup: String = "last", val chosenChannel: String = "", val density: GuideDensity = GuideDensity.STANDARD,
    val bannerSeconds: Int = 4, val wrap: Boolean = true, val browseBeforeTune: Boolean = false,
    val allChannelsSurf: Boolean = false, val showDetails: Boolean = true, val miniOverlay: Boolean = true,
    val highContrast: Boolean = false, val reducedMotion: Boolean = false, val textScale: Float = 1f,
    val buffer: BufferPreset = BufferPreset.BALANCED, val frameRateMatching: Boolean = false,
    val audioLanguage: String = "", val captionLanguage: String = "", val captions: Boolean = false,
    val captionScale: Float = 1f, val aspect: String = "fit", val hints: Boolean = true,
    val leftShortcut: String = "previous", val rightShortcut: String = "mini",
    val channelOverrides: Map<String, String> = emptyMap()
) {
    fun json(): JSONObject = JSONObject().apply {
		put("lineupId", lineupId); put("groupId", groupId); put("favoriteList", favoriteList); put("category", category)
        put("startup", startup); put("chosenChannel", chosenChannel); put("density", density.name)
        put("bannerSeconds", bannerSeconds); put("wrap", wrap); put("browseBeforeTune", browseBeforeTune)
        put("allChannelsSurf", allChannelsSurf); put("showDetails", showDetails); put("miniOverlay", miniOverlay)
        put("highContrast", highContrast); put("reducedMotion", reducedMotion); put("textScale", textScale)
        put("buffer", buffer.name); put("frameRateMatching", frameRateMatching)
        put("audioLanguage", audioLanguage); put("captionLanguage", captionLanguage); put("captions", captions)
        put("captionScale", captionScale); put("aspect", aspect); put("hints", hints)
        put("leftShortcut", leftShortcut); put("rightShortcut", rightShortcut)
        put("channelOverrides", JSONObject(channelOverrides))
    }
    companion object {
        fun parse(j: JSONObject) = TvSettings(
			lineupId = j.optLong("lineupId"), groupId = j.optLong("groupId"), favoriteList = j.optString("favoriteList"), category = j.optString("category"),
            startup = j.optString("startup", "last"), chosenChannel = j.optString("chosenChannel"),
            density = runCatching { GuideDensity.valueOf(j.optString("density")) }.getOrDefault(GuideDensity.STANDARD),
            bannerSeconds = j.optInt("bannerSeconds", 4).coerceIn(2, 6), wrap = j.optBoolean("wrap", true),
            browseBeforeTune = j.optBoolean("browseBeforeTune"), allChannelsSurf = j.optBoolean("allChannelsSurf"),
            showDetails = j.optBoolean("showDetails", true), miniOverlay = j.optBoolean("miniOverlay", true),
            highContrast = j.optBoolean("highContrast"), reducedMotion = j.optBoolean("reducedMotion"),
            textScale = j.optDouble("textScale", 1.0).toFloat().coerceIn(0.85f, 1.4f),
            buffer = runCatching { BufferPreset.valueOf(j.optString("buffer")) }.getOrDefault(BufferPreset.BALANCED),
            frameRateMatching = j.optBoolean("frameRateMatching"), audioLanguage = j.optString("audioLanguage"),
            captionLanguage = j.optString("captionLanguage"), captions = j.optBoolean("captions"),
            captionScale = j.optDouble("captionScale", 1.0).toFloat().coerceIn(0.75f, 2f),
            aspect = j.optString("aspect", "fit"), hints = j.optBoolean("hints", true),
            leftShortcut = j.optString("leftShortcut", "previous"), rightShortcut = j.optString("rightShortcut", "mini"),
            channelOverrides = j.optJSONObject("channelOverrides")?.let { obj -> obj.keys().asSequence().associateWith { obj.getString(it) } } ?: emptyMap()
        )
    }
}

class TvSettingsStore(private val context: Context, private val namespace: String) {
    private fun key(name: String) = stringPreferencesKey("$namespace|$name")
    val settings: Flow<TvSettings> = context.tvPreferences.data.map {
        runCatching { TvSettings.parse(JSONObject(it[key("settings")] ?: "{}")) }.getOrDefault(TvSettings())
    }
    val lastChannel: Flow<String?> = context.tvPreferences.data.map { it[key("last")] }
    val recents: Flow<List<String>> = context.tvPreferences.data.map { it[key("recents")]?.split('|')?.filter(String::isNotBlank) ?: emptyList() }
    suspend fun save(settings: TvSettings) { context.tvPreferences.edit { it[key("settings")] = settings.json().toString() } }
    suspend fun watched(channel: String) { context.tvPreferences.edit {
        it[key("last")] = channel
        val previous = it[key("recents")]?.split('|') ?: emptyList()
        it[key("recents")] = (listOf(channel) + previous.filter { old -> old != channel }).take(10).joinToString("|")
    } }
    suspend fun clearHistory() { context.tvPreferences.edit { it.remove(key("recents")) } }
    suspend fun resetAppearance() = save(TvSettings())
}
