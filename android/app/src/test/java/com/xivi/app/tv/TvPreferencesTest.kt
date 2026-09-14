package com.xivi.app.tv

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.time.Instant

class TvPreferencesTest {
    @Test fun accountListsRoundTripWithoutChangingLineupIdentities() {
        val before = ViewerPreferences(7, listOf(FavoriteList("sports", "Sports", listOf("1:9", "2:9"))), setOf("1:2"), listOf("2:9", "1:9"))
        assertEquals(before, ViewerPreferences.parse(JSONObject(before.json().toString())))
        val channels = listOf(TvChannel(9, 1, 1, "A", 1, "", "", "/stream/hls/a"), TvChannel(9, 2, 2, "B", 2, "", "", "/stream/hls/b"), TvChannel(2, 1, 3, "Hidden", 1, "", "", "/stream/hls/c"))
        assertEquals(listOf("2:9", "1:9"), before.arrange(channels).map { it.key })
    }
    @Test fun missingAndMalformedListingsStillLeaveTheChannelPlayable() {
        val json = JSONObject().put("id", 8).put("number", 8).put("name", "No EPG").put("stream_url", "/stream/hls/test")
        val channel = TvChannel.parse(json, 3)
        assertEquals("3:8", channel.key); assertEquals("/stream/hls/test", channel.streamUrl); assertTrue(channel.programmes.isEmpty())
    }
    @Test fun focusUsesInstantsAcrossMidnightAndDaylightSavingOverlap() {
        val first = Instant.parse("2026-11-01T05:30:00Z").toEpochMilli()
        val second = Instant.parse("2026-11-01T06:30:00Z").toEpochMilli()
        val c = TvChannel(1, 1, 1, "DST", 1, "", "", "/stream/hls/test", listOf(TvProgramme(1, "First 1:30", first, second), TvProgramme(2, "Second 1:30", second, second + 3_600_000)))
        assertEquals(1L, c.at(first)?.id); assertEquals(2L, c.at(second)?.id)
        assertEquals(second, GuideFocus(c.key, first).moveTime(c, 1).time)
    }
    @Test fun derSignaturesBecomeFixedWidthJoseCoordinates() {
        val r = byteArrayOf(0) + ByteArray(32) { 0x80.toByte() }
        val s = ByteArray(31) { 1 }
        val body = byteArrayOf(2, r.size.toByte()) + r + byteArrayOf(2, s.size.toByte()) + s
        val signature = TvDeviceKeys.joseSignature(byteArrayOf(0x30, body.size.toByte()) + body)
        assertEquals(64, signature.size); assertEquals(0x80.toByte(), signature[0]); assertEquals(0.toByte(), signature[32]); assertEquals(1.toByte(), signature[63])
    }
}
