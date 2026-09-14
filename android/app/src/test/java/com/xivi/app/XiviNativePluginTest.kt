package com.xivi.app

import com.getcapacitor.JSObject
import com.getcapacitor.PluginCall
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class XiviNativePluginTest {
    @Test
    fun playbackAcceptsIdsDecodedFromJavascriptJson() {
        val call = RecordingCall(JSObject("""{
            "channelId": 42, "lineupId": 1, "name": "News",
            "streamUrl": "/stream/hls/42", "programme": "Headlines",
            "logoUrl": "/images/news.png"
        }"""))

        val item = XiviNativePlugin().playbackItem(call)

        assertEquals(42L, item.channelId)
        assertEquals(1L, item.lineupId)
        assertEquals("News", item.name)
        assertEquals("Headlines", item.programme)
        assertEquals("/images/news.png", item.logoUrl)
        assertEquals("/stream/hls/42", item.streamUrl)
    }

    @Test
    fun playbackPreservesIdsBeyondTheIntegerRange() {
        val call = RecordingCall(JSObject("""{
            "channelId": 4294967296, "lineupId": 2147483648,
            "streamUrl": "/stream/hls/large"
        }"""))

        val item = XiviNativePlugin().playbackItem(call)

        assertEquals(4294967296L, item.channelId)
        assertEquals(2147483648L, item.lineupId)
        assertEquals("Live channel", item.name)
        assertNull(item.programme)
        assertNull(item.logoUrl)
    }

    @Test
    fun invalidPlaybackRequestsRejectWithoutEscapingThePlugin() {
        val cases = listOf(
            """{"lineupId":1,"streamUrl":"/stream/hls/42"}""" to "Channel id is required.",
            """{"channelId":42,"streamUrl":"/stream/hls/42"}""" to "Lineup id is required.",
            """{"channelId":"42","lineupId":1,"streamUrl":"/stream/hls/42"}""" to "Channel id is required.",
            """{"channelId":42.5,"lineupId":1,"streamUrl":"/stream/hls/42"}""" to "Channel id is required.",
            """{"channelId":42,"lineupId":1}""" to "Stream URL is required."
        )

        for ((json, expectedMessage) in cases) {
            val call = RecordingCall(JSObject(json))

            XiviNativePlugin().playVideo(call)

            assertFalse(call.resolved)
            assertEquals(expectedMessage, call.rejectionMessage)
            assertNotNull(call.rejection)
        }
    }

    private class RecordingCall(data: JSObject) : PluginCall(null, "XiviNative", "test", "playVideo", data) {
        var resolved = false
        var rejectionMessage: String? = null
        var rejection: Exception? = null

        override fun resolve() {
            resolved = true
        }

        override fun reject(message: String, error: Exception) {
            rejectionMessage = message
            rejection = error
        }
    }
}
