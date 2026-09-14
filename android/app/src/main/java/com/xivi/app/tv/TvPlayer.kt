package com.xivi.app.tv

import android.content.Context
import android.os.SystemClock
import androidx.media3.common.*
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.LoadControl
import androidx.media3.exoplayer.analytics.AnalyticsListener
import androidx.media3.exoplayer.analytics.PlayerId
import androidx.media3.exoplayer.source.MediaSource
import androidx.media3.exoplayer.source.TrackGroupArray
import androidx.media3.exoplayer.trackselection.ExoTrackSelection
import androidx.media3.exoplayer.source.LoadEventInfo
import androidx.media3.exoplayer.source.MediaLoadData
import androidx.media3.datasource.HttpDataSource
import androidx.media3.session.MediaSession
import com.xivi.app.NativePlayback
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.json.JSONObject
import java.util.UUID

@UnstableApi
private class TvLoadControl(private val base: LoadControl = DefaultLoadControl.Builder()
    .setBufferDurationsMs(5_000, 15_000, 1_000, 2_000).setTargetBufferBytes(32 * 1024 * 1024).build()) : LoadControl by base {
    @Volatile var preset = BufferPreset.BALANCED
    // Kotlin delegation does not forward Media3's Java default methods. Forward
    // the player-scoped API explicitly; otherwise its defaults throw at runtime.
    override fun onPrepared(playerId: PlayerId) = base.onPrepared(playerId)
    override fun onTracksSelected(parameters: LoadControl.Parameters, trackGroups: TrackGroupArray, trackSelections: Array<out ExoTrackSelection?>) =
        base.onTracksSelected(parameters, trackGroups, trackSelections)
    override fun onStopped(playerId: PlayerId) = base.onStopped(playerId)
    override fun onReleased(playerId: PlayerId) = base.onReleased(playerId)
    override fun getBackBufferDurationUs(playerId: PlayerId) = base.getBackBufferDurationUs(playerId)
    override fun retainBackBufferFromKeyframe(playerId: PlayerId) = base.retainBackBufferFromKeyframe(playerId)
    override fun shouldContinueLoading(parameters: LoadControl.Parameters) = base.shouldContinueLoading(parameters)
    override fun shouldContinuePreloading(playerId: PlayerId, timeline: Timeline, mediaPeriodId: MediaSource.MediaPeriodId, bufferedDurationUs: Long) = false
    override fun shouldStartPlayback(parameters: LoadControl.Parameters): Boolean {
        var threshold = preset.startupMs * 1000L * if (parameters.rebuffering) 2 else 1
        if (parameters.targetLiveOffsetUs != C.TIME_UNSET) threshold = minOf(threshold, parameters.targetLiveOffsetUs / 2)
        return parameters.bufferedDurationUs / parameters.playbackSpeed >= threshold || base.getAllocator(parameters.playerId).totalBytesAllocated >= 32 * 1024 * 1024
    }
}

data class TvPlaybackStatus(val channel: TvChannel? = null, val loading: Boolean = false, val playing: Boolean = false,
    val message: String = "", val firstFrameMs: Long? = null, val identity: String = "", val tracks: Tracks = Tracks.EMPTY)

@UnstableApi
class TvPlayer(context: Context, private val auth: TvAuthRepository) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val io = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val releaseMutex = Mutex()
    private val pendingRelease = linkedMapOf<String, String>()
    private val loadControl = TvLoadControl()
    val player = NativePlayback.create(context, auth, loadControl)
    private val session = MediaSession.Builder(context, player).setId("xivi-tv-playback").build()
    val status = MutableStateFlow(TvPlaybackStatus())
    var onWatched: (TvChannel) -> Unit = {}
    private var generation = 0L
    private var tuneJob: Job? = null
    private var recoveryJob: Job? = null
    private var requestAt = 0L
    private var manifestMs = 0L
    private var requestedAt = ""
    private var rebufferAt = 0L
    private var recoveryAt = 0L
    private var pausedAt = 0L
    private var retryCount = 0
    private var configuredTracks: Triple<String, String, Boolean>? = null

    init {
        player.videoChangeFrameRateStrategy = C.VIDEO_CHANGE_FRAME_RATE_STRATEGY_OFF
        player.addAnalyticsListener(object : AnalyticsListener {
            override fun onLoadCompleted(eventTime: AnalyticsListener.EventTime, loadEventInfo: LoadEventInfo, mediaLoadData: MediaLoadData) {
                if (mediaLoadData.dataType == C.DATA_TYPE_MANIFEST && manifestMs == 0L && loadEventInfo.uri.getQueryParameter("playback_id") == status.value.identity) {
                    manifestMs = SystemClock.elapsedRealtime() - requestAt
                    event("tv_tune_request", manifestMs)
                }
            }
            private fun current(eventTime: AnalyticsListener.EventTime): Boolean {
                val timeline = eventTime.timeline
                return !timeline.isEmpty && eventTime.windowIndex < timeline.windowCount &&
                    timeline.getWindow(eventTime.windowIndex, Timeline.Window()).mediaItem.mediaId == status.value.identity
            }
            override fun onRenderedFirstFrame(eventTime: AnalyticsListener.EventTime, output: Any, renderTimeMs: Long) {
                if (!current(eventTime)) return
                if (status.value.firstFrameMs == null) {
                    val elapsed = SystemClock.elapsedRealtime() - requestAt
                    status.value = status.value.copy(firstFrameMs = elapsed, loading = false, message = "")
                    status.value.channel?.let(onWatched)
                    event("tv_first_frame", elapsed)
                }
                if (recoveryAt > 0) { event("tv_recovery", SystemClock.elapsedRealtime() - recoveryAt); recoveryAt = 0 }
                retryCount = 0
            }
            override fun onPlaybackStateChanged(eventTime: AnalyticsListener.EventTime, playbackState: Int) {
                if (!current(eventTime)) return
                val loading = playbackState == Player.STATE_BUFFERING
                if (loading && status.value.firstFrameMs != null && rebufferAt == 0L) rebufferAt = SystemClock.elapsedRealtime()
                if (playbackState == Player.STATE_READY && rebufferAt > 0) { event("tv_rebuffer", SystemClock.elapsedRealtime() - rebufferAt); rebufferAt = 0 }
                status.value = status.value.copy(loading = loading, playing = player.isPlaying)
            }
            override fun onPlayerError(eventTime: AnalyticsListener.EventTime, error: PlaybackException) {
                if (!current(eventTime)) return
                status.value = status.value.copy(loading = false, playing = false, message = "Playback interrupted. Reconnecting…")
                if (auth.state.value == TvAuthState.REVOKED) { status.value = status.value.copy(message = "This TV pairing was revoked. Sign in on your phone."); return }
                val httpError = generateSequence<Throwable>(error) { it.cause }.filterIsInstance<HttpDataSource.InvalidResponseCodeException>().firstOrNull()
                if (httpError?.responseCode in listOf(403, 404, 410)) { status.value = status.value.copy(message = "This channel is no longer available. Choose another channel in the guide."); return }
                val expected = generation; recoveryJob?.cancel()
                if (recoveryAt == 0L) recoveryAt = SystemClock.elapsedRealtime()
                recoveryJob = scope.launch {
                    delay(minOf(15_000L, 1000L shl retryCount.coerceAtMost(4))); retryCount++
                    if (expected != generation) return@launch
                    if (error.errorCode == PlaybackException.ERROR_CODE_BEHIND_LIVE_WINDOW) {
                        player.seekToDefaultPosition(); status.value = status.value.copy(message = "The paused programme left the live window. Resuming live.")
                    }
                    player.prepare(); player.play()
                }
            }
        })
        player.addListener(object : Player.Listener {
            override fun onIsPlayingChanged(isPlaying: Boolean) { status.value = status.value.copy(playing = isPlaying) }
            override fun onTracksChanged(tracks: Tracks) { status.value = status.value.copy(tracks = tracks) }
        })
    }
    fun configure(settings: TvSettings) {
        loadControl.preset = settings.buffer
        player.videoChangeFrameRateStrategy = if (settings.frameRateMatching) C.VIDEO_CHANGE_FRAME_RATE_STRATEGY_ONLY_IF_SEAMLESS else C.VIDEO_CHANGE_FRAME_RATE_STRATEGY_OFF
        val tracks = Triple(settings.audioLanguage, settings.captionLanguage, settings.captions)
        if (tracks != configuredTracks) {
            configuredTracks = tracks
            player.trackSelectionParameters = player.trackSelectionParameters.buildUpon().clearOverrides()
                .setPreferredAudioLanguage(settings.audioLanguage.ifBlank { null })
                .setPreferredTextLanguage(settings.captionLanguage.ifBlank { null })
                .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, !settings.captions).build()
        }
    }
    fun tune(channel: TvChannel) {
        if (status.value.channel?.key == channel.key && status.value.identity.isNotBlank() && player.playerError == null) return
        generation++; val expected = generation; tuneJob?.cancel(); recoveryJob?.cancel()
        relinquishCurrent(); player.stop(); player.clearMediaItems()
        player.trackSelectionParameters = player.trackSelectionParameters.buildUpon().clearOverrides().build()
        val identity = UUID.randomUUID().toString()
        requestAt = SystemClock.elapsedRealtime(); requestedAt = java.time.Instant.now().toString(); manifestMs = 0; rebufferAt = 0; recoveryAt = 0; retryCount = 0
        status.value = TvPlaybackStatus(channel, loading = true, identity = identity)
        tuneJob = scope.launch {
            try {
            if (!releaseMutex.withLock { flushReleases() }) throw java.io.IOException("Previous playback release is still pending")
            if (generation != expected) return@launch
            val url = auth.absoluteUrl(channel.streamUrl).toHttpUrl().newBuilder()
                .setQueryParameter("playback_id", identity).setQueryParameter("viewer_id", identity).build()
            player.setMediaItem(MediaItem.Builder().setMediaId(identity).setUri(url.toString())
                .setMimeType(MimeTypes.APPLICATION_M3U8)
                .setMediaMetadata(MediaMetadata.Builder().setTitle(channel.name).setSubtitle(channel.at(System.currentTimeMillis())?.title ?: "Live").setIsPlayable(true).build())
                .setLiveConfiguration(MediaItem.LiveConfiguration.Builder().setMaxPlaybackSpeed(1.02f).build()).build())
            player.prepare(); player.play()
            } catch (e: CancellationException) { throw e }
            catch (_: Exception) { if (generation == expected) status.value = status.value.copy(loading = false, message = "This channel could not be started. Check the server connection.") }
        }
    }
    private fun relinquishCurrent() {
        val previous = status.value
        if (previous.identity.isNotBlank() && previous.channel != null) {
            pendingRelease[previous.identity] = streamId(previous.channel)
            if (previous.firstFrameMs == null) event("tv_tune_cancelled", SystemClock.elapsedRealtime() - requestAt)
        }
    }
    private suspend fun flushReleases(): Boolean {
        val releases = pendingRelease.toMap()
        // Release completes before the next source request. Cancellation must not
        // open a one-connection provider before the old tune has been surrendered.
        val completed = withContext(Dispatchers.IO + NonCancellable) {
            val completed = mutableSetOf<String>()
            for ((identity, stream) in releases) {
                for (attempt in 0..2) {
                    try {
                        auth.json("/api/v2/watch/playback/release", "POST", JSONObject().put("playback_id", identity).put("stream_id", stream))
                        completed.add(identity); break
                    } catch (e: Exception) {
                        // Revocation has already invalidated the server viewer. Never
                        // discard identities on a proxy/network failure or tune over them.
                        if (auth.state.value == TvAuthState.REVOKED) { completed.add(identity); break }
                        if (e is TvApiException && e.status == 404 && e.code == "stream_not_found") { completed.add(identity); break }
                        if (attempt < 2) delay(250L shl attempt)
                    }
                }
            }
            completed
        }
        completed.forEach(pendingRelease::remove)
        return pendingRelease.isEmpty()
    }
    fun stop() {
        generation++; tuneJob?.cancel(); recoveryJob?.cancel(); relinquishCurrent()
        player.stop(); player.clearMediaItems(); status.value = TvPlaybackStatus()
        scope.launch { releaseMutex.withLock { flushReleases() } }
    }
    fun pauseOrResume() {
        if (player.playWhenReady) { pausedAt = SystemClock.elapsedRealtime(); player.pause() }
        else {
            if (pausedAt > 0 && player.duration > 0 && SystemClock.elapsedRealtime() - pausedAt >= player.duration) {
                player.seekToDefaultPosition(); status.value = status.value.copy(message = "The pause exceeded the live window. Resuming live.")
            }
            pausedAt = 0; player.play()
        }
    }
    fun selectTrack(group: Tracks.Group, index: Int) {
        if (!group.isTrackSupported(index)) return
        player.trackSelectionParameters = player.trackSelectionParameters.buildUpon()
            .setTrackTypeDisabled(group.type, false).setOverrideForType(TrackSelectionOverride(group.mediaTrackGroup, index)).build()
    }
    fun close() {
        stop(); session.release(); player.release()
        scope.launch { releaseMutex.withLock { flushReleases() }; scope.cancel(); io.cancel() }
    }
    private fun event(code: String, elapsed: Long) {
        val snapshot = status.value; val channel = snapshot.channel ?: return
        val timing = JSONObject().put("duration_ms", elapsed).put("tune_id", snapshot.identity)
            .put("tune_requested_at", requestedAt).put("manifest_ready_ms", manifestMs)
            .put("buffer_preset", loadControl.preset.name)
        if (code == "tv_first_frame" && manifestMs > 0) timing.put("post_manifest_first_frame_ms", (elapsed - manifestMs).coerceAtLeast(0))
        io.launch { runCatching { auth.json("/api/v2/stream/telemetry", "POST", JSONObject()
            .put("stream_id", streamId(channel)).put("viewer_id", snapshot.identity).put("code", code).put("severity", "info")
            .put("message", "Android TV playback measurement").put("details", timing)) } }
    }
    private fun streamId(channel: TvChannel) = channel.streamUrl.substringBefore('?').trimEnd('/').substringAfterLast('/')
}
