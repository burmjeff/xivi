package com.xivi.app

import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import org.json.JSONObject
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors

@CapacitorPlugin(name = "XiviNative")
class XiviNativePlugin : Plugin() {
    private val executor: ExecutorService = Executors.newCachedThreadPool()
    private lateinit var repository: AuthRepository
    private var removePlaybackObserver: (() -> Unit)? = null

    override fun load() {
        repository = AuthRepository.get(context)
        removePlaybackObserver = PlaybackCoordinator.observe { state ->
            notifyListeners("playbackState", JSObject(state.toJson().toString()), true)
        }
    }

    override fun handleOnDestroy() {
        removePlaybackObserver?.invoke()
        executor.shutdown()
        super.handleOnDestroy()
    }

    @PluginMethod
    fun getServer(call: PluginCall) {
        call.resolve(JSObject().put("url", repository.server() ?: JSONObject.NULL))
    }

    @PluginMethod
    fun setServer(call: PluginCall) = background(call) {
        val raw = call.getString("url") ?: throw IllegalArgumentException("Enter an HTTPS Xivi server.")
        JSObject().put("url", repository.setServer(raw))
    }

    @PluginMethod
    fun clearServer(call: PluginCall) {
        repository.clearServer()
        call.resolve()
    }

    @PluginMethod
    fun request(call: PluginCall) = background(call) {
        val path = call.getString("path") ?: throw IllegalArgumentException("A request path is required.")
        val method = call.getString("method") ?: "GET"
        val headerObject = call.getObject("headers") ?: JSObject()
        val headers = mutableMapOf<String, String>()
        headerObject.keys().forEach { key -> headers[key] = headerObject.optString(key) }
        val response = repository.request(path, method, headers, call.getString("body"))
        responseObject(response)
    }

    @PluginMethod
    fun login(call: PluginCall) = background(call) {
        responseObject(repository.request("/api/v2/auth/login", "POST", suppliedBody = requiredBody(call)))
    }

    @PluginMethod
    fun completeMfa(call: PluginCall) = background(call) {
        responseObject(repository.request("/api/v2/auth/login/mfa", "POST", suppliedBody = requiredBody(call)))
    }

    @PluginMethod
    fun refreshSession(call: PluginCall) = background(call) {
        JSObject().put("authenticated", repository.refresh(true))
    }

    @PluginMethod
    fun logout(call: PluginCall) = background(call) {
        val response = repository.request("/api/v2/mobile/auth/logout", "POST", suppliedBody = "{}")
        if (response.status !in 200..299 && response.status != 401) {
            throw IllegalStateException("The mobile session could not be ended.")
        }
        JSObject()
    }

    @PluginMethod
    fun cacheArtwork(call: PluginCall) = background(call) {
        val path = call.getString("path") ?: throw IllegalArgumentException("Artwork path is required.")
        JSObject().put("url", ArtworkCache.dataUrl(context, repository, path))
    }

    @PluginMethod
    fun playVideo(call: PluginCall) {
        PlaybackCoordinator.play(context, playbackItem(call), audioOnly = false, openPlayer = true)
        call.resolve()
    }

    @PluginMethod
    fun playAudio(call: PluginCall) {
        PlaybackCoordinator.play(context, playbackItem(call), audioOnly = true, openPlayer = false)
        call.resolve()
    }

    @PluginMethod
    fun reopenPlayer(call: PluginCall) {
        PlaybackCoordinator.reopen(context)
        call.resolve()
    }

    @PluginMethod
    fun stopPlayback(call: PluginCall) {
        PlaybackCoordinator.stop(context)
        call.resolve()
    }

    @PluginMethod
    fun getPlaybackState(call: PluginCall) {
        call.resolve(JSObject(PlaybackCoordinator.state().toJson().toString()))
    }

    private fun playbackItem(call: PluginCall) = PlaybackItem(
        call.getLong("channelId") ?: throw IllegalArgumentException("Channel id is required."),
        call.getLong("lineupId") ?: throw IllegalArgumentException("Lineup id is required."),
        call.getString("name") ?: "Live channel",
        call.getString("programme"),
        call.getString("logoUrl"),
        call.getString("streamUrl") ?: throw IllegalArgumentException("Stream URL is required."),
        call.getString("audioStreamUrl")
    )

    private fun requiredBody(call: PluginCall): String =
        call.getString("body") ?: throw IllegalArgumentException("A JSON request body is required.")

    private fun responseObject(response: NativeHttpResponse): JSObject {
        val responseHeaders = JSObject()
        response.headers.forEach { (name, value) -> responseHeaders.put(name, value) }
        return JSObject()
            .put("status", response.status)
            .put("headers", responseHeaders)
            .put("body", response.body)
    }

    private fun background(call: PluginCall, operation: () -> JSObject) {
        executor.execute {
            try {
                call.resolve(operation())
            } catch (error: Exception) {
                call.reject(error.message ?: "The native operation failed.", error)
            }
        }
    }
}
