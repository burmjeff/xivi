package com.xivi.app.tv

import android.content.Context
import android.util.Base64
import androidx.test.platform.app.InstrumentationRegistry
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.math.BigInteger
import java.security.AlgorithmParameters
import java.security.KeyFactory
import java.security.KeyStore
import java.security.MessageDigest
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import java.time.Instant
import java.util.UUID
import java.util.concurrent.Callable
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

/** Real Android Keystore, mocked HTTPS transport. Never changes production TV preferences. */
class TvAuthenticationTest {
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private class Server {
        val renewals = AtomicInteger()
        val proofs = java.util.concurrent.ConcurrentHashMap.newKeySet<String>()
        var failRenewal = false
        var revoked = false
        var expireFirstAccess = false
        val client = OkHttpClient.Builder().addInterceptor { chain ->
            val request = chain.request(); var status = 200
            val result = when (request.url.encodedPath) {
                "/healthz" -> JSONObject().put("status", "ok").put("tv_api_version", 1)
                "/api/v2/tv/auth/device" -> {
                    verify(request.header("DPoP")!!, request.method, request.url.toString(), null)
                    JSONObject().put("device_code", "test-code").put("user_code", "TEST-CODE")
                }
                "/api/v2/tv/auth/token" -> {
                    verify(request.header("DPoP")!!, request.method, request.url.toString(), null)
                    val n = renewals.incrementAndGet()
                    if (failRenewal || revoked) {
                        status = if (revoked) 400 else 503
                        JSONObject().put("code", if (revoked) "invalid_grant" else "proxy_unavailable").put("message", "Test failure")
                    } else JSONObject().put("token_type", "DPoP").put("refresh_token", "xtr_stable_test_credential")
                        .put("access_token", "xta_test_$n").put("access_expires_at", Instant.now().plusSeconds(900).toString())
                        .put("principal", JSONObject().put("user_id", 42))
                }
                else -> {
                    val authorization = request.header("Authorization") ?: error("DPoP authorization missing")
                    assertTrue(authorization.startsWith("DPoP "))
                    val token = authorization.removePrefix("DPoP ")
                    verify(request.header("DPoP")!!, request.method, request.url.toString(), token)
                    if (expireFirstAccess && token == "xta_test_1") { status = 401; JSONObject().put("code", "invalid_token") }
                    else JSONObject().put("ok", true)
                }
            }
            Response.Builder().request(request).protocol(Protocol.HTTP_1_1).code(status).message("Test")
                .body(result.toString().toResponseBody("application/json".toMediaType())).build()
        }.build()
        private fun verify(proof: String, method: String, target: String, access: String?) {
            val parts = proof.split('.'); assertEquals(3, parts.size)
            fun decode(value: String) = Base64.decode(value, Base64.URL_SAFE or Base64.NO_WRAP)
            val header = JSONObject(String(decode(parts[0]))); val claims = JSONObject(String(decode(parts[1])))
            assertEquals("dpop+jwt", header.getString("typ")); assertEquals("ES256", header.getString("alg"))
            assertEquals(method, claims.getString("htm")); assertEquals(target.substringBefore('?'), claims.getString("htu"))
            assertTrue("Proof replay", proofs.add(claims.getString("jti")))
            if (access != null) assertEquals(MessageDigest.getInstance("SHA-256").digest(access.toByteArray()).base64Url(), claims.getString("ath"))
            val jwk = header.getJSONObject("jwk")
            val parameters = AlgorithmParameters.getInstance("EC").apply { init(ECGenParameterSpec("secp256r1")) }.getParameterSpec(ECParameterSpec::class.java)
            val publicKey = KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(ECPoint(BigInteger(1, decode(jwk.getString("x"))), BigInteger(1, decode(jwk.getString("y")))), parameters))
            val raw = decode(parts[2]); assertEquals(64, raw.size)
            fun integer(bytes: ByteArray): ByteArray { val positive = BigInteger(1, bytes).toByteArray(); return byteArrayOf(2, positive.size.toByte()) + positive }
            val body = integer(raw.copyOfRange(0, 32)) + integer(raw.copyOfRange(32, 64))
            val der = byteArrayOf(0x30, body.size.toByte()) + body
            val verifier = Signature.getInstance("SHA256withECDSA").apply { initVerify(publicKey); update("${parts[0]}.${parts[1]}".toByteArray()) }
            assertTrue("Invalid Android DPoP signature", verifier.verify(der))
        }
    }
    private fun test(block: (TvAuthRepository, Server, String) -> Unit) {
        val file = "tv_auth_test_${UUID.randomUUID()}"; val server = Server()
        val repository = TvAuthRepository(context, server.client, file)
        val origin = "https://tv-test-${UUID.randomUUID()}.example"
        try {
            repository.configure(origin); repository.beginPairing(); assertTrue(repository.poll("test-code"))
            block(repository, server, file)
        } finally {
            context.getSharedPreferences(file, Context.MODE_PRIVATE).edit().clear().commit()
            val keys = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
            val alias = "xivi.tv.${origin.sha256().take(32)}"
            keys.deleteEntry(alias); keys.deleteEntry("$alias.storage")
        }
    }
    @Test fun credentialIsEncryptedAndProcessRestartRenewsWithoutRePairing() = test { repository, server, file ->
        val stored = context.getSharedPreferences(file, Context.MODE_PRIVATE).getString("credential", "")!!
        assertFalse(stored.contains("xtr_stable")); assertTrue(repository.hasCredential())
        val restarted = TvAuthRepository(context, server.client, file)
        restarted.renew(false)
        assertEquals(TvAuthState.AUTHENTICATED, restarted.state.value); assertEquals(2, server.renewals.get())
    }
    @Test fun simultaneousSegmentArtworkAndUIFailuresRenewOnlyOnce() = test { repository, server, _ ->
        server.expireFirstAccess = true
        val executor = Executors.newFixedThreadPool(8)
        try {
            val work = (1..16).map { Callable { repository.json("/stream/hls/channel/segment$it.ts?viewer_id=test").getBoolean("ok") } }
            executor.invokeAll(work).forEach { assertTrue(it.get(15, TimeUnit.SECONDS)) }
            assertEquals("Concurrent 401s rotated/repeated renewal", 2, server.renewals.get())
        } finally { executor.shutdownNow() }
    }
    @Test fun serverErrorsNeverDeletePairingButRevocationIsDistinct() = test { repository, server, _ ->
        server.failRenewal = true
        assertTrue(runCatching { repository.renew(true) }.isFailure)
        assertTrue(repository.hasCredential()); assertEquals(TvAuthState.DISCONNECTED, repository.state.value)
        server.failRenewal = false; repository.renew(true); assertEquals(TvAuthState.AUTHENTICATED, repository.state.value)
        server.revoked = true
        assertTrue(runCatching { repository.renew(true) }.isFailure)
        assertEquals(TvAuthState.REVOKED, repository.state.value)
    }
    @Test fun unsafeOriginsAreRejectedWithoutDiscardingTheSavedCredential() = test { repository, _, _ ->
        assertTrue(runCatching { repository.configure("http://example.com") }.isFailure)
        assertTrue(runCatching { repository.configure("https://example.com/path") }.isFailure)
        assertTrue(runCatching { repository.absoluteUrl("https://unrelated.example/stream/hls/a") }.isFailure)
        assertTrue(repository.hasCredential())
    }
}
