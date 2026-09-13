package com.xivi.app

import android.app.PendingIntent
import android.content.Intent
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
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
            override fun onPlaybackStateChanged(playbackState: Int) = publishState()
        })
        val activityIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
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

    override fun onDestroy() {
        session.release()
        player.release()
        super.onDestroy()
    }

    private fun play(item: PlaybackItem) {
        currentItem = item
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
            .setUri(auth.absoluteUrl(item.streamUrl))
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
            if (item == null) {
                PlaybackState()
            } else {
                PlaybackState(
                    active = player.playbackState != Player.STATE_IDLE,
                    channelId = item.channelId,
                    name = item.name,
                    programme = item.programme,
                    playing = player.isPlaying || player.playWhenReady
                )
            }
        )
    }
}
