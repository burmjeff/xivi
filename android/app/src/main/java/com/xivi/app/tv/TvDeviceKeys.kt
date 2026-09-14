package com.xivi.app.tv

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONObject
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.PrivateKey
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

internal fun ByteArray.base64Url(): String = Base64.encodeToString(this, Base64.NO_WRAP or Base64.NO_PADDING or Base64.URL_SAFE)
internal fun String.sha256() = MessageDigest.getInstance("SHA-256").digest(toByteArray()).joinToString("") { "%02x".format(it) }

class TvDeviceKeys(server: String) {
    private val alias = "xivi.tv.${server.sha256().take(32)}"
    private val store get() = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
    fun exists() = store.containsAlias(alias)
    fun credentialKeyExists() = store.containsAlias("$alias.storage")
    fun create() {
        if (exists()) return
        KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore").apply {
            initialize(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY)
                .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                .setDigests(KeyProperties.DIGEST_SHA256).build())
            generateKeyPair()
        }
    }
    // No StrongBox requirement or biometric gate: unattended TV renewal must work.
    fun proof(method: String, target: String, access: String?, nonce: String?, now: Long): String {
        check(exists()) { "TV security key is missing. Pair this TV again." }
        val key = store.getCertificate(alias).publicKey as ECPublicKey
        fun coordinate(bytes: ByteArray) = bytes.takeLast(32).toByteArray().let { ByteArray(32 - it.size) + it }.base64Url()
        val jwk = JSONObject().put("kty", "EC").put("crv", "P-256")
            .put("x", coordinate(key.w.affineX.toByteArray())).put("y", coordinate(key.w.affineY.toByteArray()))
        val header = JSONObject().put("typ", "dpop+jwt").put("alg", "ES256").put("jwk", jwk)
        val claims = JSONObject().put("jti", UUID.randomUUID().toString()).put("htm", method)
            .put("htu", target.substringBefore('?').substringBefore('#')).put("iat", now / 1000)
        if (nonce != null) claims.put("nonce", nonce)
        if (access != null) claims.put("ath", MessageDigest.getInstance("SHA-256").digest(access.toByteArray()).base64Url())
        val input = header.toString().toByteArray().base64Url() + "." + claims.toString().toByteArray().base64Url()
        val signer = Signature.getInstance("SHA256withECDSA").apply { initSign(store.getKey(alias, null) as PrivateKey); update(input.toByteArray()) }
        return input + "." + joseSignature(signer.sign()).base64Url()
    }
    private fun secret(create: Boolean): SecretKey {
        (store.getKey("$alias.storage", null) as? SecretKey)?.let { return it }
        check(create) { "TV encrypted credential key is missing. Pair this TV again." }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder("$alias.storage", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    fun encrypt(value: String): String {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, secret(true)) }
        return (cipher.iv + cipher.doFinal(value.toByteArray())).base64Url()
    }
    fun decrypt(value: String): String {
        val bytes = Base64.decode(value, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
        require(bytes.size > 28)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.DECRYPT_MODE, secret(false), GCMParameterSpec(128, bytes.copyOfRange(0, 12))) }
        return String(cipher.doFinal(bytes.copyOfRange(12, bytes.size)))
    }
    companion object {
        /** Android emits ASN.1 DER ECDSA; JOSE requires exactly 32-byte r || s. */
        fun joseSignature(der: ByteArray): ByteArray {
            require(der.size in 8..72 && der[0] == 0x30.toByte() && der[1].toInt() == der.size - 2)
            var offset = 2
            fun integer(): ByteArray {
                require(der[offset++] == 2.toByte())
                val size = der[offset++].toInt() and 255
                require(size in 1..33 && offset + size <= der.size)
                var bytes = der.copyOfRange(offset, offset + size); offset += size
                if (bytes.size == 33) { require(bytes[0] == 0.toByte()); bytes = bytes.drop(1).toByteArray() }
                return ByteArray(32 - bytes.size) + bytes
            }
            val result = integer() + integer(); require(offset == der.size); return result
        }
    }
}
