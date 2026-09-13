package com.xivi.app

import android.content.Context
import android.os.Build
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.time.Instant
import java.util.concurrent.TimeUnit

data class NativeHttpResponse(
    val status: Int,
    val headers: Map<String, String>,
    val body: String
)

class AuthRepository private constructor(context: Context) {
    private val applicationContext = context.applicationContext
    private val tokens = SecureTokenStore(applicationContext)
    private val refreshLock = Any()
    @Volatile private var accessToken: String? = tokens.accessToken()
    @Volatile private var accessExpiryMillis: Long = tokens.accessExpiryMillis()

    private val baseClient = OkHttpClient.Builder()
        .connectTimeout(12, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .callTimeout(40, TimeUnit.SECONDS)
        .followRedirects(false)
        .followSslRedirects(false)
        .build()

    private val authenticatedClient = baseClient.newBuilder()
        .addInterceptor { chain ->
            if (accessToken.isNullOrBlank() || accessExpiryMillis <= System.currentTimeMillis() + 30_000L) {
                refresh(false)
            }
            val original = chain.request()
            val firstToken = accessToken
            val first = chain.proceed(
                original.newBuilder().apply {
                    if (!firstToken.isNullOrBlank()) header("Authorization", "Bearer $firstToken")
                }.build()
            )
            if (first.code != 401 || !refresh(true, firstToken)) return@addInterceptor first
            first.close()
            chain.proceed(
                original.newBuilder().header("Authorization", "Bearer ${accessToken.orEmpty()}").build()
            )
        }
        .build()

    fun server(): String? = tokens.server()

    fun hasRefreshCredential(): Boolean =
        !tokens.refreshToken().isNullOrBlank() && tokens.refreshExpiryMillis() > System.currentTimeMillis()

    fun setServer(raw: String): String {
        val normalized = normalizeServer(raw)
        val request = Request.Builder().url("$normalized/healthz").get().build()
        baseClient.newCall(request).execute().use { response ->
            if (!response.isSuccessful) throw IllegalArgumentException("That server did not return a healthy Xivi response.")
            val body = response.body?.string().orEmpty()
            val health = runCatching { JSONObject(body) }.getOrNull()
                ?: throw IllegalArgumentException("That address is not a Xivi server.")
            if (health.optString("status") != "ok") {
                throw IllegalArgumentException("That server did not return a healthy Xivi response.")
            }
            if (!MobileApiContract.healthIsCompatible(
                    health.optString("status"),
                    health.optInt("mobile_api_version", 0)
                )
            ) {
                throw IllegalArgumentException("Update and restart the Xivi server before connecting the Android app.")
            }
        }
        if (tokens.server() != normalized) {
            clearSession()
            ArtworkCache.clear(applicationContext)
        }
        tokens.setServer(normalized)
        return normalized
    }

    fun clearServer() {
        clearSession()
        ArtworkCache.clear(applicationContext)
        tokens.setServer(null)
    }

    fun absoluteUrl(path: String): String {
		val root = server() ?: throw IllegalStateException("Sign in on your phone")
		val rootUrl = root.toHttpUrl()
		val parsed = path.toHttpUrlOrNullSafe()
		if (parsed != null) {
			require(parsed.isHttps && parsed.host == rootUrl.host && parsed.port == rootUrl.port) { "Cross-server media URL rejected" }
            return parsed.toString()
        }
        require(path.startsWith('/')) { "Invalid server path" }
        return root + path
    }

    fun mediaClient(): OkHttpClient = authenticatedClient

    fun download(path: String): Pair<ByteArray, String> {
        val request = Request.Builder().url(absoluteUrl(path)).header("Accept", "image/*").get().build()
        authenticatedClient.newCall(request).execute().use { response ->
            require(response.isSuccessful) { "Artwork is unavailable" }
            val bytes = response.body?.bytes() ?: ByteArray(0)
            require(bytes.isNotEmpty()) { "Artwork is empty" }
            return bytes to (response.header("Content-Type")?.substringBefore(';') ?: "image/png")
        }
    }

    fun request(
        path: String,
        method: String,
        suppliedHeaders: Map<String, String> = emptyMap(),
        suppliedBody: String? = null
    ): NativeHttpResponse {
        val mapping = mapWebAuthRequest(path, suppliedBody)
        val publicRequest = mapping.path == "/api/v2/mobile/auth/login" ||
            mapping.path == "/api/v2/mobile/auth/login/mfa" ||
            mapping.path == "/api/v2/mobile/auth/refresh" ||
            mapping.path == "/api/v2/auth/bootstrap-status"
        val client = if (publicRequest) baseClient else authenticatedClient
        val builder = Request.Builder().url(absoluteUrl(mapping.path))
        suppliedHeaders.forEach { (name, value) ->
            if (!name.equals("authorization", true) && !name.equals("host", true) && !name.equals("cookie", true)) {
                builder.header(name, value)
            }
        }
        val requestBody = mapping.body?.toRequestBody("application/json; charset=utf-8".toMediaType())
        builder.method(method.uppercase(), if (method.equals("GET", true) || method.equals("HEAD", true)) null else requestBody)
        val response = client.newCall(builder.build()).execute()
        response.use {
            var status = response.code
            var body = response.body?.string().orEmpty()
            if (response.isSuccessful && body.isNotBlank()) {
                body = persistEnvelope(body, mapping.unwrapPrincipal)
            }
            val errorCode = if (body.isBlank()) null else runCatching {
                JSONObject(body).optString("code").ifBlank { null }
            }.getOrNull()
            if (MobileApiContract.loginRequiresServerUpdate(mapping.mobileLogin, status, errorCode)) {
                status = 426
                body = JSONObject()
                    .put("code", "mobile_api_unavailable")
                    .put("message", "Update and restart the Xivi server before signing in with the Android app.")
                    .put("retryable", false)
                    .toString()
            }
            if (mapping.logout && response.code < 500) clearSession()
            return NativeHttpResponse(
                status,
                response.headers.toMultimap().mapValues { it.value.joinToString(", ") },
                body
            )
        }
    }

    fun refresh(force: Boolean, failedToken: String? = null): Boolean = synchronized(refreshLock) {
        val now = System.currentTimeMillis()
        if (failedToken != null && accessToken != failedToken && accessExpiryMillis > now + 30_000L) return true
        if (!force && !accessToken.isNullOrBlank() && accessExpiryMillis > now + 30_000L) return true
        val refresh = tokens.refreshToken()
        if (refresh.isNullOrBlank() || tokens.refreshExpiryMillis() <= now) {
            clearSession()
            return false
        }
        val root = server() ?: return false
        val body = JSONObject().put("refresh_token", refresh).toString()
            .toRequestBody("application/json; charset=utf-8".toMediaType())
        val request = Request.Builder().url("$root/api/v2/mobile/auth/refresh").post(body).build()
        return try {
            baseClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    if (response.code == 401) clearSession()
                    false
                } else {
                    persistEnvelope(response.body?.string().orEmpty(), false)
                    true
                }
            }
        } catch (_: Exception) {
            false
        }
    }

    private fun persistEnvelope(body: String, unwrapPrincipal: Boolean): String {
        val json = JSONObject(body)
        if (!json.has("access_token")) return body
        val access = json.getString("access_token")
        val refresh = json.getString("refresh_token")
        val accessExpiry = Instant.parse(json.getString("access_expires_at")).toEpochMilli()
        val refreshExpiry = Instant.parse(json.getString("refresh_expires_at")).toEpochMilli()
        tokens.saveSession(access, accessExpiry, refresh, refreshExpiry)
        accessToken = access
        accessExpiryMillis = accessExpiry
        return if (unwrapPrincipal) json.getJSONObject("principal").toString() else body
    }

    private fun clearSession() {
        accessToken = null
        accessExpiryMillis = 0L
        tokens.clearSession()
    }

    private data class RequestMapping(
        val path: String,
        val body: String?,
        val unwrapPrincipal: Boolean = false,
        val logout: Boolean = false,
        val mobileLogin: Boolean = false
    )

    private fun mapWebAuthRequest(path: String, body: String?): RequestMapping {
        val (route, query) = path.split('?', limit = 2).let { it.first() to it.getOrNull(1) }
        val mapped = when (route) {
            "/api/v2/auth/login" -> "/api/v2/mobile/auth/login"
            "/api/v2/auth/login/mfa" -> "/api/v2/mobile/auth/login/mfa"
            "/api/v2/auth/password" -> "/api/v2/mobile/auth/password"
            "/api/v2/auth/logout" -> "/api/v2/mobile/auth/logout"
            else -> route
        }
        var mappedBody = body
        val unwrap = mapped != route && mapped != "/api/v2/mobile/auth/logout"
        if ((mapped == "/api/v2/mobile/auth/login" || mapped == "/api/v2/mobile/auth/login/mfa") && !body.isNullOrBlank()) {
            mappedBody = JSONObject(body).apply {
                remove("trust_browser")
                put("device_name", deviceName())
            }.toString()
        }
        val fullPath = if (query.isNullOrBlank()) mapped else "$mapped?$query"
        return RequestMapping(
            fullPath,
            mappedBody,
            unwrap,
            mapped == "/api/v2/mobile/auth/logout",
            mapped == "/api/v2/mobile/auth/login" || mapped == "/api/v2/mobile/auth/login/mfa"
        )
    }

    private fun normalizeServer(raw: String): String {
        var value = raw.trim()
        if (!value.contains("://")) value = "https://$value"
        val url = value.toHttpUrl()
        require(url.isHttps) { "Xivi mobile requires HTTPS." }
        require(url.username.isEmpty() && url.password.isEmpty()) { "Credentials are not allowed in the server address." }
        require(url.query == null && url.fragment == null && url.encodedPath == "/") { "Enter only the server scheme and host." }
        return url.newBuilder().encodedPath("/").build().toString().removeSuffix("/")
    }

    private fun deviceName(): String = listOf(Build.MANUFACTURER, Build.MODEL)
        .filter { it.isNotBlank() }
        .joinToString(" ")
        .take(80)
        .ifBlank { "Android device" }

    companion object {
        @Volatile private var instance: AuthRepository? = null

        fun get(context: Context): AuthRepository = instance ?: synchronized(this) {
            instance ?: AuthRepository(context).also { instance = it }
        }
    }
}

private fun String.toHttpUrlOrNullSafe() = runCatching { toHttpUrl() }.getOrNull()
