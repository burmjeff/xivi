package com.xivi.app.tv

import org.junit.Assert.*
import org.junit.Test

class GuideFocusTest {
    private fun channel(id: Long, programmes: List<TvProgramme>) = TvChannel(id, 1, id, "Channel $id", 1, "Group", "", "/stream/hls/$id", programmes)
    @Test fun movingVerticallyPreservesInstantAcrossUnequalProgrammes() {
        val channels = listOf(channel(1, listOf(TvProgramme(1, "Long", 0, 120))),
            channel(2, listOf(TvProgramme(2, "Short", 0, 30), TvProgramme(3, "Later", 30, 120))))
        val focus = GuideFocus("1:1", 60).moveChannel(channels, 1)
        assertEquals(60L, focus.time)
        assertEquals("Later", channels[1].at(focus.time)?.title)
    }
    @Test fun gapsRemainNavigableAndBoundaryBelongsToNextProgramme() {
        val c = channel(1, listOf(TvProgramme(1, "A", 0, 30), TvProgramme(2, "B", 30, 60)))
        assertEquals("B", c.at(30)?.title)
        assertNull(c.at(90))
        assertEquals("1:1", GuideFocus("1:1", 90).moveChannel(listOf(c), 1).channelKey)
    }
    @Test fun duplicateTimesAreDeterministicAndWrappingIsExplicit() {
        val a = channel(1, listOf(TvProgramme(2, "B", 0, 90), TvProgramme(1, "A", 0, 90)))
        val b = channel(2, emptyList())
        assertEquals("A", a.at(40)?.title)
        assertEquals("1:2", GuideFocus("1:2").moveChannel(listOf(a, b), 1).channelKey)
        assertEquals("1:1", GuideFocus("1:2").moveChannel(listOf(a, b), 1, true).channelKey)
    }
    @Test fun fiveThousandChannelsHaveStableIdentity() {
        val channels = (1L..5000L).map { channel(it, emptyList()) }
        assertEquals("1:5000", GuideFocus("1:4999", 123).moveChannel(channels, 1).channelKey)
    }
}
