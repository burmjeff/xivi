package com.xivi.app.tv

import android.content.Context
import android.os.Build
import com.xivi.app.NativeApi
import com.xivi.app.NativeHttpResponse
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException
import java.time.Instant
import java.util.concurrent.TimeUnit

class TvApiException(val status: Int, val code: String, message: String) : IOException(message)

/** The sole authentication owner for TV UI, player and artwork. Never deletes
 * pairing in response to transport/clock/server errors or ordinary HTTP 401. */
class TvAuthRepository internal constructor(context: Context, testClient: OkHttpClient? = null, preferenceFile: String = "xivi_tv_identity") : NativeApi {
    private val prefs = context.getSharedPreferences(preferenceFile, Context.MODE_PRIVATE)
    private val lock = Any()
    @Volatile private var access: String? = null
    @Volatile private var expiry = 0L
    private var nonce: String? = null
    private val mutableState = MutableStateFlow(if (server() == null) TvAuthState.UNCONFIGURED else if (prefs.contains("credential")) TvAuthState.CONNECTING else TvAuthState.UNPAIRED)
    val state = mutableState.asStateFlow()
    private val base = testClient ?: OkHttpClient.Builder().followRedirects(false).followSslRedirects(false)
        .connectTimeout(12, TimeUnit.SECONDS).readTimeout(25, TimeUnit.SECONDS).callTimeout(35, TimeUnit.SECONDS).build()
    private val client = base.newBuilder().apply { interceptors().add(0, okhttp3.Interceptor { chain ->
        val original = chain.request()
        fun signed(token: String) = original.newBuilder().header("Authorization", "DPoP $token")
            .header("DPoP", keys().proof(original.method, original.url.toString(), token, null, System.currentTimeMillis())).build()
        // Validate origin and snapshot its token/key atomically with server
        // changes. Never attach a new server's identity to an in-flight old URL.
        val (firstToken, firstRequest) = synchronized(lock) {
            absoluteUrl(original.url.toString())
            renew(false)
            val token = access ?: throw IOException("Waiting for TV connection")
            token to signed(token)
        }
        val first = chain.proceed(firstRequest)
        if (first.code != 401) return@Interceptor first
        val body = runCatching { JSONObject(first.peekBody(8192).string()) }.getOrNull()
        when (body?.optString("code")) {
            "pairing_revoked" -> synchronized(lock) { if (access == firstToken) mutableState.value = TvAuthState.REVOKED }
            "invalid_token" -> {
                first.close()
                val retry = synchronized(lock) {
                    absoluteUrl(original.url.toString())
                    renew(true, firstToken)
                    signed(access ?: throw IOException("Waiting for TV connection"))
                }
                return@Interceptor chain.proceed(retry)
            }
        }
        first
    }) }.build()

    override fun server(): String? = prefs.getString("server", null)
    private fun keys() = TvDeviceKeys(server() ?: error("Configure a server first"))
    override fun cacheNamespace() = "${server()}|${prefs.getLong("user_id", 0)}".sha256()
    fun hasCredential() = prefs.contains("credential")
    fun configure(raw: String): String = synchronized(lock) {
        val parsed = (if (raw.contains("://")) raw.trim() else "https://${raw.trim()}").toHttpUrl()
        require(parsed.isHttps && parsed.username.isEmpty() && parsed.password.isEmpty() && parsed.encodedPath == "/" && parsed.query == null && parsed.fragment == null) { "Enter a trusted HTTPS server hostname, without a path." }
        val normalized = parsed.toString().trimEnd('/')
        base.newCall(Request.Builder().url("$normalized/healthz").build()).execute().use {
            require(it.isSuccessful) { "Xivi server is unavailable." }
            val health = JSONObject(it.body?.string().orEmpty())
            require(health.optString("status") == "ok" && health.optInt("tv_api_version") == 1) { "Update the Xivi server to a TV-compatible version." }
        }
        if (server() != normalized) {
            check(prefs.edit().clear().putString("server", normalized).commit()) { "Could not save TV server" }
            access = null; expiry = 0; nonce = null
        }
        mutableState.value = if (hasCredential()) TvAuthState.CONNECTING else TvAuthState.UNPAIRED
        normalized
    }
    override fun absoluteUrl(path: String): String {
        val root = (server() ?: throw IOException("Configure the server on this TV")).toHttpUrl()
        val url = if (path.startsWith('/')) root.resolve(path) ?: error("Invalid URL") else path.toHttpUrl()
        require(url.isHttps && url.host == root.host && url.port == root.port && url.username.isEmpty() && url.password.isEmpty()) { "Cross-server TV request rejected" }
        return url.toString()
    }
    override fun mediaClient() = client
    override fun request(path: String, method: String, suppliedHeaders: Map<String, String>, suppliedBody: String?): NativeHttpResponse {
        val builder = Request.Builder().url(absoluteUrl(path))
        suppliedHeaders.filterKeys { it.lowercase() !in setOf("authorization", "dpop", "host", "cookie") }.forEach { (k,v) -> builder.header(k,v) }
        builder.method(method, if (method in listOf("GET", "HEAD")) null else (suppliedBody ?: "{}").toRequestBody(JSON))
        return client.newCall(builder.build()).execute().use { NativeHttpResponse(it.code, it.headers.toMultimap().mapValues { h -> h.value.joinToString(",") }, it.body?.string().orEmpty()) }
    }
    fun json(path: String, method: String = "GET", body: JSONObject? = null): JSONObject {
        val r = request(path, method, emptyMap(), body?.toString())
        val json = runCatching { JSONObject(r.body) }.getOrElse { JSONObject() }
        if (r.status !in 200..299) throw TvApiException(r.status, json.optString("code"), json.optString("message", "Server unavailable (${r.status})"))
        return json
    }
    override fun download(path: String): Pair<ByteArray, String> = client.newCall(Request.Builder().url(absoluteUrl(path)).build()).execute().use {
        if (!it.isSuccessful) throw IOException("Artwork unavailable")
        val source = it.body ?: throw IOException("Artwork empty")
        require(source.contentLength() <= 4 * 1024 * 1024) { "Artwork too large" }
        val output = java.io.ByteArrayOutputStream()
        val buffer = ByteArray(8192)
        source.byteStream().use { input ->
            while (true) { val n = input.read(buffer); if (n < 0) break; require(output.size() + n <= 4 * 1024 * 1024); output.write(buffer, 0, n) }
        }
        val bytes = output.toByteArray()
        bytes to (it.header("Content-Type") ?: "image/png")
    }
    fun beginPairing(): JSONObject = synchronized(lock) {
        keys().create()
        tokenRequest("/api/v2/tv/auth/device", JSONObject().put("device_name", "${Build.MANUFACTURER} ${Build.MODEL}".take(80)))
    }
    fun poll(code: String): Boolean = synchronized(lock) {
        try {
            persist(tokenRequest("/api/v2/tv/auth/token", JSONObject().put("grant_type", "urn:ietf:params:oauth:grant-type:device_code").put("device_code", code)))
            true
        } catch (e: TvApiException) { if (e.code in listOf("authorization_pending", "slow_down")) false else throw e }
    }
    fun renew(force: Boolean, failedToken: String? = null) = synchronized(lock) {
        if (failedToken != null && failedToken != access && expiry > System.currentTimeMillis() + 30_000) return@synchronized
        if (!force && access != null && expiry > System.currentTimeMillis() + 30_000) return@synchronized
        val cipher = prefs.getString("credential", null) ?: throw IOException("Pair this TV on your phone")
        try {
            if (!keys().exists() || !keys().credentialKeyExists()) { mutableState.value = TvAuthState.REVOKED; throw IOException("TV security key is missing. Pair this TV again.") }
            val refresh = keys().decrypt(cipher)
            persist(tokenRequest("/api/v2/tv/auth/token", JSONObject().put("grant_type", "refresh_token").put("refresh_token", refresh)))
        } catch (e: Exception) {
            if (e is TvApiException && e.code == "invalid_grant") mutableState.value = TvAuthState.REVOKED
            else if (mutableState.value != TvAuthState.REVOKED) mutableState.value = TvAuthState.DISCONNECTED
            throw if (e is IOException) e else IOException("Unable to unlock TV pairing. Check the device security key.", e)
        }
    }
    private fun tokenRequest(path: String, body: JSONObject): JSONObject {
        repeat(3) {
            val url = absoluteUrl(path)
            val req = Request.Builder().url(url).post(body.toString().toRequestBody(JSON))
                .header("DPoP", keys().proof("POST", url, null, nonce, System.currentTimeMillis())).build()
            base.newCall(req).execute().use { response ->
                val json = runCatching { JSONObject(response.body?.string().orEmpty()) }.getOrElse { JSONObject() }
                val nextNonce = response.header("DPoP-Nonce")
                if (json.optString("code") == "use_dpop_nonce" && !nextNonce.isNullOrEmpty()) { nonce = nextNonce; return@use }
                if (!response.isSuccessful) throw TvApiException(response.code, json.optString("code"), json.optString("message", "Server connection unavailable"))
                return json
            }
        }
        throw IOException("The TV security challenge could not be completed")
    }
    private fun persist(json: JSONObject) {
        require(json.getString("token_type") == "DPoP")
        val credential = json.getString("refresh_token")
        // A synchronous encrypted commit precedes the paired state. Access is
        // memory-only; a restart silently obtains another short-lived token.
        check(prefs.edit().putString("credential", keys().encrypt(credential))
            .putLong("user_id", json.getJSONObject("principal").getLong("user_id")).commit()) { "Could not save TV pairing" }
        access = json.getString("access_token"); expiry = Instant.parse(json.getString("access_expires_at")).toEpochMilli()
        mutableState.value = TvAuthState.AUTHENTICATED
    }
    fun logout() = synchronized(lock) {
        json("/api/v2/tv/auth/logout", "POST") // No local-only sign-out disguised as revocation.
        check(prefs.edit().remove("credential").remove("user_id").commit())
        access = null; expiry = 0; mutableState.value = TvAuthState.UNPAIRED
    }
    companion object {
        private val JSON = "application/json; charset=utf-8".toMediaType()
        @Volatile private var instance: TvAuthRepository? = null
        fun get(context: Context) = instance ?: synchronized(this) { instance ?: TvAuthRepository(context.applicationContext).also { instance = it } }
    }
}
