package com.xivi.app

import android.net.Uri
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.MimeTypes

internal fun PlaybackItem.toMediaItem(streamUri: String, artworkUri: Uri? = null): MediaItem =
    MediaItem.Builder()
        .setMediaId("channel:$lineupId:$channelId")
        .setUri(streamUri)
        // Xivi's /stream/hls/{id} endpoint does not have a .m3u8 extension.
        .setMimeType(MimeTypes.APPLICATION_M3U8)
        .setLiveConfiguration(MediaItem.LiveConfiguration.Builder().setMaxPlaybackSpeed(1.02f).build())
        .setMediaMetadata(
            MediaMetadata.Builder()
                .setTitle(name)
                .setSubtitle(programme ?: "Live")
                .setArtist("Xivi")
                .setIsPlayable(true)
                .setIsBrowsable(false)
                .setArtworkUri(artworkUri)
                .build()
        )
        .build()
