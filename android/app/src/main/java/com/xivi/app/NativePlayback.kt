package com.xivi.app

import android.content.Context
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.LoadControl
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory

/** Shared native edge; activities and services retain their own lifecycles. */
@UnstableApi
object NativePlayback {
    fun create(context: Context, api: NativeApi, loadControl: LoadControl? = null): ExoPlayer {
        val builder = ExoPlayer.Builder(context)
            .setMediaSourceFactory(DefaultMediaSourceFactory(OkHttpDataSource.Factory(api.mediaClient()).setUserAgent("Xivi Android/${BuildConfig.VERSION_NAME}")))
            .setAudioAttributes(AudioAttributes.Builder().setContentType(C.AUDIO_CONTENT_TYPE_MOVIE).setUsage(C.USAGE_MEDIA).build(), true)
            .setHandleAudioBecomingNoisy(true).setWakeMode(C.WAKE_MODE_NETWORK)
        if (loadControl != null) builder.setLoadControl(loadControl)
        return builder.build()
    }
}
