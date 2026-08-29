package com.xivi.app

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Bundle
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.session.LibraryResult
import androidx.media3.session.MediaConstants
import androidx.media3.session.MediaLibraryService
import androidx.media3.session.MediaSession
import androidx.core.content.ContextCompat
import com.google.common.collect.ImmutableList
import com.google.common.util.concurrent.Futures
import com.google.common.util.concurrent.ListenableFuture
import com.google.common.util.concurrent.MoreExecutors
import org.json.JSONObject
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors

@UnstableApi
class XiviMediaLibraryService : MediaLibraryService() {
    private lateinit var player: ExoPlayer
    private lateinit var session: MediaLibrarySession
    private lateinit var auth: AuthRepository
    private lateinit var browse: BrowseRepository
    private val libraryExecutor = MoreExecutors.listeningDecorator(Executors.newFixedThreadPool(2))
    private val items = ConcurrentHashMap<String, MediaItem>()
    private val channelDetails = ConcurrentHashMap<String, CarChannel>()
    private val searchResults = ConcurrentHashMap<String, List<MediaItem>>()
    private val rootLimits = ConcurrentHashMap<String, Int>()
    private var currentItem: PlaybackItem? = null
    private var audioOnly = false
    private var noisyRegistered = false

    private val noisyReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action == android.media.AudioManager.ACTION_AUDIO_BECOMING_NOISY) player.pause()
        }
    }

    override fun onCreate() {
        super.onCreate()
        auth = AuthRepository.get(this)
        browse = BrowseRepository(this, auth)
        val dataSource = OkHttpDataSource.Factory(auth.mediaClient())
            .setUserAgent("Xivi Android/${BuildConfig.VERSION_NAME}")
        player = ExoPlayer.Builder(this)
            .setMediaSourceFactory(DefaultMediaSourceFactory(dataSource))
            .setAudioAttributes(
                AudioAttributes.Builder().setContentType(C.AUDIO_CONTENT_TYPE_MOVIE).setUsage(C.USAGE_MEDIA).build(),
                true
            )
            .setHandleAudioBecomingNoisy(true)
            .setWakeMode(C.WAKE_MODE_NETWORK)
            .build()
        player.addListener(object : Player.Listener {
            override fun onIsPlayingChanged(isPlaying: Boolean) = publishState()
            override fun onPlaybackStateChanged(playbackState: Int) = publishState()
        })
        val activityIntent = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        session = MediaLibrarySession.Builder(this, player, LibraryCallback())
            .setSessionActivity(activityIntent)
            .setId("xivi-live-library")
            .build()
        ContextCompat.registerReceiver(
            this,
            noisyReceiver,
            IntentFilter(android.media.AudioManager.ACTION_AUDIO_BECOMING_NOISY),
            ContextCompat.RECEIVER_NOT_EXPORTED
        )
        noisyRegistered = true
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            PlaybackCoordinator.ACTION_PLAY -> {
                val item = intent.getStringExtra(PlaybackCoordinator.EXTRA_ITEM)
                    ?.let { PlaybackItem.fromJson(JSONObject(it)) }
                    ?: PlaybackCoordinator.restore(this)
                if (item != null) play(item, intent.getBooleanExtra(PlaybackCoordinator.EXTRA_AUDIO_ONLY, false))
            }
            PlaybackCoordinator.ACTION_STOP -> {
                player.stop()
                player.clearMediaItems()
                currentItem = null
                audioOnly = false
                PlaybackCoordinator.update(PlaybackState())
                stopSelf()
            }
        }
        return super.onStartCommand(intent, flags, startId)
    }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaLibrarySession = session

    override fun onDestroy() {
        if (noisyRegistered) unregisterReceiver(noisyReceiver)
        session.release()
        player.release()
        libraryExecutor.shutdown()
        super.onDestroy()
    }

    private fun play(item: PlaybackItem, audio: Boolean) {
        val path = if (audio) item.audioStreamUrl else item.streamUrl
        if (path.isNullOrBlank()) return
        currentItem = item
        audioOnly = audio
        val metadata = MediaMetadata.Builder()
            .setTitle(item.name)
            .setSubtitle(item.programme ?: "Live")
            .setArtist("Xivi")
            .setIsPlayable(true)
            .setIsBrowsable(false)
            .setArtworkUri(ArtworkCache.contentUri(this, auth, item.logoUrl))
            .build()
        val mediaItem = MediaItem.Builder()
            .setMediaId("channel:${item.lineupId}:${item.channelId}")
            .setUri(auth.absoluteUrl(path))
            .setLiveConfiguration(MediaItem.LiveConfiguration.Builder().setMaxPlaybackSpeed(1.02f).build())
            .setMediaMetadata(metadata)
            .build()
        player.setMediaItem(mediaItem)
        player.prepare()
        player.playWhenReady = true
        publishState()
    }

    private fun publishState() {
        val item = currentItem
        PlaybackCoordinator.update(
            if (item == null) PlaybackState() else PlaybackState(
                active = player.playbackState != Player.STATE_IDLE,
                channelId = item.channelId,
                name = item.name,
                programme = item.programme,
                playing = player.isPlaying || player.playWhenReady,
                audioOnly = audioOnly
            )
        )
    }

    private inner class LibraryCallback : MediaLibrarySession.Callback {
        override fun onConnect(session: MediaSession, controller: MediaSession.ControllerInfo): MediaSession.ConnectionResult {
            if (isCarController(controller.packageName)) PlaybackCoordinator.switchVideoToCarAudio(this@XiviMediaLibraryService)
            return super.onConnect(session, controller)
        }

        override fun onGetLibraryRoot(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            params: LibraryParams?
		): ListenableFuture<LibraryResult<MediaItem>> {
			rootLimits[browser.packageName] = params?.extras?.getInt(MediaConstants.EXTRAS_KEY_ROOT_CHILDREN_LIMIT, 100)
				?.takeIf { it > 0 } ?: 100
			return Futures.immediateFuture(LibraryResult.ofItem(folder(ROOT, "Xivi", "Live audio channels"), params))
		}

        override fun onGetChildren(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            parentId: String,
            page: Int,
            pageSize: Int,
            params: LibraryParams?
        ): ListenableFuture<LibraryResult<ImmutableList<MediaItem>>> = libraryExecutor.submit<LibraryResult<ImmutableList<MediaItem>>> {
            val all = runCatching { children(parentId, rootLimit(browser, params)) }
                .getOrElse { listOf(signInOrUnavailable(it)) }
            val paged = page(all, page, pageSize)
            LibraryResult.ofItemList(paged, params)
        }

        override fun onGetItem(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            mediaId: String
        ): ListenableFuture<LibraryResult<MediaItem>> = libraryExecutor.submit<LibraryResult<MediaItem>> {
            val item = items[mediaId] ?: runCatching { resolveItem(mediaId) }.getOrElse { signInOrUnavailable(it) }
            LibraryResult.ofItem(item, null)
        }

        override fun onSearch(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            query: String,
            params: LibraryParams?
        ): ListenableFuture<LibraryResult<Void>> = libraryExecutor.submit<LibraryResult<Void>> {
            searchResults[query] = runCatching { browse.search(query).map(::channel) }.getOrDefault(emptyList())
            session.notifySearchResultChanged(browser, query, searchResults[query]?.size ?: 0, params)
            LibraryResult.ofVoid(params)
        }

        override fun onGetSearchResult(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            query: String,
            page: Int,
            pageSize: Int,
            params: LibraryParams?
        ): ListenableFuture<LibraryResult<ImmutableList<MediaItem>>> = libraryExecutor.submit<LibraryResult<ImmutableList<MediaItem>>> {
            val result = searchResults[query] ?: runCatching { browse.search(query).map(::channel) }.getOrDefault(emptyList())
            LibraryResult.ofItemList(page(result, page, pageSize), params)
        }

        override fun onAddMediaItems(
            mediaSession: MediaSession,
            controller: MediaSession.ControllerInfo,
			mediaItems: List<MediaItem>
		): ListenableFuture<List<MediaItem>> = libraryExecutor.submit<List<MediaItem>> {
			val resolved = mediaItems.map { requested -> items[requested.mediaId] ?: resolveItem(requested.mediaId) }
			resolved.firstOrNull()?.mediaId?.let { mediaId ->
				channelDetails[mediaId]?.let { details ->
					currentItem = PlaybackItem(details.id, details.lineupId, details.name, details.programme,
						details.logoUrl, details.audioStreamUrl.orEmpty(), details.audioStreamUrl)
					audioOnly = true
				}
			}
			resolved
        }
    }

    private fun children(parentId: String, rootLimit: Int): List<MediaItem> {
        if (!auth.hasRefreshCredential()) return listOf(signInItem())
        return when {
            parentId == ROOT -> {
                val lineups = browse.lineups()
                if (lineups.size <= rootLimit) lineups.map(::lineup) else listOf(folder(LINEUPS, "Lineups", "Browse all accessible lineups"))
            }
            parentId == LINEUPS -> browse.lineups().map(::lineup)
            parentId.startsWith("lineup:") -> {
                val lineupId = parentId.substringAfter(':').toLong()
                browse.groups(lineupId).map(::group)
            }
            parentId.startsWith("group:") -> {
                val parts = parentId.split(':')
                browse.channels(parts[1].toLong(), parts[2].toLong()).map(::channel)
            }
            else -> emptyList()
        }
    }

    private fun resolveItem(mediaId: String): MediaItem {
        val parts = mediaId.split(':')
        require(parts.size == 3 && parts[0] == "channel")
        return browse.channels(parts[1].toLong()).first { it.id == parts[2].toLong() }.let(::channel)
    }

    private fun lineup(lineup: CarLineup) = folder("lineup:${lineup.id}", lineup.name, "Lineup")
    private fun group(group: CarGroup) = folder("group:${group.lineupId}:${group.id}", group.name, "Channel group")

    private fun folder(id: String, title: String, subtitle: String): MediaItem = remember(
        MediaItem.Builder().setMediaId(id).setMediaMetadata(
            MediaMetadata.Builder().setTitle(title).setSubtitle(subtitle).setIsBrowsable(true).setIsPlayable(false).build()
        ).build()
    )

	private fun channel(channel: CarChannel): MediaItem {
        val end = channel.programmeEnds?.let { runCatching { END_FORMAT.format(Instant.parse(it).atZone(ZoneId.systemDefault())) }.getOrNull() }
        val subtitle = listOfNotNull(channel.programme, end?.let { "Ends $it" }).joinToString(" · ").ifBlank { "Live" }
		val mediaId = "channel:${channel.lineupId}:${channel.id}"
		channelDetails[mediaId] = channel
		val item = MediaItem.Builder()
			.setMediaId(mediaId)
            .setUri(auth.absoluteUrl(channel.audioStreamUrl!!))
            .setMediaMetadata(
                MediaMetadata.Builder()
                    .setTitle(channel.name).setSubtitle(subtitle).setArtist("Xivi · Live")
                    .setArtworkUri(ArtworkCache.contentUri(this, auth, channel.logoUrl))
                    .setIsBrowsable(false).setIsPlayable(true).build()
            ).build()
        return remember(item)
    }

    private fun remember(item: MediaItem): MediaItem = item.also { items[it.mediaId] = it }

    private fun signInItem() = folder(SIGN_IN, "Sign in on your phone", "Open Xivi on your phone to continue")
    private fun signInOrUnavailable(error: Throwable) =
        if (error is SecurityException || !auth.hasRefreshCredential()) signInItem()
        else folder(UNAVAILABLE, "Xivi is temporarily unavailable", "The last library could not be refreshed")

    private fun rootLimit(browser: MediaSession.ControllerInfo, params: LibraryParams?): Int {
		var limit = rootLimits[browser.packageName] ?: 100
		val keys = listOf("android.media.browse.extra.PAGE_SIZE", MediaConstants.EXTRAS_KEY_ROOT_CHILDREN_LIMIT)
        keys.forEach { key ->
            val candidate = params?.extras?.getInt(key, 0) ?: browser.connectionHints.getInt(key, 0)
            if (candidate > 0) limit = minOf(limit, candidate)
        }
        return limit.coerceAtLeast(1)
    }

    private fun <T> page(items: List<T>, page: Int, pageSize: Int): List<T> {
        val safeSize = pageSize.coerceIn(1, 100)
        val start = (page.coerceAtLeast(0) * safeSize).coerceAtMost(items.size)
        return items.subList(start, (start + safeSize).coerceAtMost(items.size))
    }

    private fun isCarController(packageName: String): Boolean = packageName in setOf(
        "com.google.android.projection.gearhead",
		"com.android.car.media"
    )

    companion object {
        private const val ROOT = "xivi-root"
        private const val LINEUPS = "xivi-lineups"
        private const val SIGN_IN = "xivi-sign-in"
        private const val UNAVAILABLE = "xivi-unavailable"
        private val END_FORMAT = DateTimeFormatter.ofPattern("h:mm a")
    }
}
