package com.xivi.app

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MobileApiContractTest {
    @Test
    fun healthRequiresTheAndroidApiCapability() {
        assertTrue(MobileApiContract.healthIsCompatible("ok", MobileApiContract.VERSION))
        assertFalse(MobileApiContract.healthIsCompatible("ok", 0))
        assertFalse(MobileApiContract.healthIsCompatible("unhealthy", MobileApiContract.VERSION))
    }

    @Test
    fun oldServerAuthenticationCatchAllIsRecognizedAtLogin() {
        assertTrue(
            MobileApiContract.loginRequiresServerUpdate(
                isMobileLogin = true,
                status = 401,
                errorCode = "authentication_required"
            )
        )
        assertFalse(
            MobileApiContract.loginRequiresServerUpdate(
                isMobileLogin = true,
                status = 401,
                errorCode = "invalid_credentials"
            )
        )
        assertFalse(
            MobileApiContract.loginRequiresServerUpdate(
                isMobileLogin = false,
                status = 401,
                errorCode = "authentication_required"
            )
        )
    }
}
