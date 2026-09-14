package com.xivi.app

import android.app.PendingIntent
import android.content.Intent
import android.util.Log
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService
import org.json.JSONObject

@UnstableApi
class XiviPlaybackService : MediaSessionService() {
    private lateinit var player: ExoPlayer
    private lateinit var session: MediaSession
    private lateinit var auth: AuthRepository
    private var currentItem: PlaybackItem? = null
    private var playbackError: String? = null

    override fun onCreate() {
        super.onCreate()
        auth = AuthRepository.get(this)
        val dataSource = OkHttpDataSource.Factory(auth.mediaClient())
            .setUserAgent("Xivi Android/${BuildConfig.VERSION_NAME}")
        player = ExoPlayer.Builder(this)
            .setMediaSourceFactory(DefaultMediaSourceFactory(dataSource))
            .setAudioAttributes(
                AudioAttributes.Builder()
                    .setContentType(C.AUDIO_CONTENT_TYPE_MOVIE)
                    .setUsage(C.USAGE_MEDIA)
                    .build(),
                true
            )
            .setHandleAudioBecomingNoisy(true)
            .setWakeMode(C.WAKE_MODE_NETWORK)
            .build()
        player.addListener(object : Player.Listener {
            override fun onIsPlayingChanged(isPlaying: Boolean) = publishState()
            override fun onPlaybackStateChanged(playbackState: Int) {
                if (playbackState == Player.STATE_BUFFERING || playbackState == Player.STATE_READY) playbackError = null
                publishState()
            }
            override fun onPlayerError(error: PlaybackException) {
                Log.e("XiviPlayback", "Channel playback failed: ${error.errorCodeName}", error)
                playbackError = "This channel could not be played. Retry to reconnect."
                publishState()
            }
        })
        val activityIntent = PendingIntent.getActivity(
            this,
            0,
            PlaybackCoordinator.playerIntent(this),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        session = MediaSession.Builder(this, player)
            .setSessionActivity(activityIntent)
            .setId("xivi-live-playback")
            .build()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            PlaybackCoordinator.ACTION_PLAY -> {
                val item = intent.getStringExtra(PlaybackCoordinator.EXTRA_ITEM)
                    ?.let { PlaybackItem.fromJson(JSONObject(it)) }
                    ?: PlaybackCoordinator.restore(this)
                if (item != null) play(item)
            }
            PlaybackCoordinator.ACTION_STOP -> {
                player.stop()
                player.clearMediaItems()
                currentItem = null
                PlaybackCoordinator.update(PlaybackState())
                stopSelf()
            }
        }
        return super.onStartCommand(intent, flags, startId)
    }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession = session

    override fun onTaskRemoved(rootIntent: Intent?) {
        // Swiping Xivi away must not leave an unreachable audio-only session.
        PlaybackCoordinator.stop(this)
        super.onTaskRemoved(rootIntent)
    }

    override fun onDestroy() {
        session.release()
        player.release()
        PlaybackCoordinator.update(PlaybackState())
        super.onDestroy()
    }

    private fun play(item: PlaybackItem) {
        currentItem = item
        playbackError = null
        try {
            val mediaItem = item.toMediaItem(
                auth.absoluteUrl(item.streamUrl),
                ArtworkCache.contentUri(this, auth, item.logoUrl)
            )
            player.setMediaItem(mediaItem)
            player.prepare()
            player.play()
        } catch (error: Exception) {
            Log.e("XiviPlayback", "Could not start channel playback", error)
            player.stop()
            playbackError = "This channel could not be started. Retry to reconnect."
        }
        publishState()
    }

    private fun publishState() {
        val item = currentItem
        PlaybackCoordinator.update(
            if (item == null) {
                PlaybackState()
            } else {
                PlaybackState(
                    active = true,
                    channelId = item.channelId,
                    name = item.name,
                    programme = item.programme,
                    playing = player.isPlaying,
                    loading = player.playbackState == Player.STATE_BUFFERING,
                    error = playbackError,
                    lineupId = item.lineupId
                )
            }
        )
    }
}
