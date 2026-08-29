package com.xivi.app

import org.junit.Assert.assertFalse
import org.junit.Test

class ParkedVideoCapabilityTest {
    @Test
    fun baseApplicationNeverAdvertisesParkedVideo() {
        assertFalse(DisabledParkedVideoCapability.enabled)
    }
}
