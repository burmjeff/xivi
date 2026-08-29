package com.xivi.app

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class SecureTokenStore(context: Context) {
    private val preferences = context.getSharedPreferences("xivi_mobile", Context.MODE_PRIVATE)
    private val keyAlias = "xivi.mobile.session.v1"

    @Synchronized
    fun server(): String? = preferences.getString("server", null)

    @Synchronized
    fun setServer(value: String?) {
        preferences.edit().apply {
            if (value == null) remove("server") else putString("server", value)
        }.apply()
    }

    @Synchronized
    fun accessToken(): String? = decrypt(preferences.getString("access_token", null))

    @Synchronized
    fun refreshToken(): String? = decrypt(preferences.getString("refresh_token", null))

    @Synchronized
    fun accessExpiryMillis(): Long = preferences.getLong("access_expiry", 0L)

    @Synchronized
    fun refreshExpiryMillis(): Long = preferences.getLong("refresh_expiry", 0L)

    @Synchronized
    fun saveSession(access: String, accessExpiry: Long, refresh: String, refreshExpiry: Long) {
        preferences.edit()
            .putString("access_token", encrypt(access))
            .putString("refresh_token", encrypt(refresh))
            .putLong("access_expiry", accessExpiry)
            .putLong("refresh_expiry", refreshExpiry)
            .apply()
    }

    @Synchronized
    fun clearSession() {
        preferences.edit()
            .remove("access_token")
            .remove("refresh_token")
            .remove("access_expiry")
            .remove("refresh_expiry")
            .apply()
    }

    private fun secretKey(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(keyAlias, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        generator.init(
            KeyGenParameterSpec.Builder(
                keyAlias,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setRandomizedEncryptionRequired(true)
                .build()
        )
        return generator.generateKey()
    }

    private fun encrypt(value: String): String {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, secretKey())
        val iv = Base64.encodeToString(cipher.iv, Base64.NO_WRAP)
        val payload = Base64.encodeToString(cipher.doFinal(value.toByteArray(Charsets.UTF_8)), Base64.NO_WRAP)
        return "$iv.$payload"
    }

    private fun decrypt(value: String?): String? {
        if (value.isNullOrBlank()) return null
        return runCatching {
            val parts = value.split('.', limit = 2)
            require(parts.size == 2)
            val iv = Base64.decode(parts[0], Base64.NO_WRAP)
            val payload = Base64.decode(parts[1], Base64.NO_WRAP)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, secretKey(), GCMParameterSpec(128, iv))
            String(cipher.doFinal(payload), Charsets.UTF_8)
        }.getOrNull()
    }
}
