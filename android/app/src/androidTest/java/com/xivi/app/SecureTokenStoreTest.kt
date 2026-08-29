package com.xivi.app

import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SecureTokenStoreTest {
    @Test
    fun tokensRoundTripThroughAndroidKeystoreAndCanBeCleared() {
        val store = SecureTokenStore(ApplicationProvider.getApplicationContext())
        store.clearSession()
        store.saveSession("xma_test", 1234L, "xmr_test", 5678L)
        assertEquals("xma_test", store.accessToken())
        assertEquals("xmr_test", store.refreshToken())
        store.clearSession()
        assertNull(store.accessToken())
        assertNull(store.refreshToken())
    }
}
