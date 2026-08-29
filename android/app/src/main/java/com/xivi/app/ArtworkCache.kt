package com.xivi.app

import android.content.Context
import android.net.Uri
import android.util.Base64
import java.io.File
import java.security.MessageDigest

object ArtworkCache {
    const val AUTHORITY = "com.xivi.app.artwork"

    private fun directory(context: Context) = File(context.cacheDir, "artwork").apply { mkdirs() }

    fun dataUrl(context: Context, repository: AuthRepository, path: String): String {
        val entry = ensure(context, repository, path)
        val mime = File(entry.parentFile, "${entry.name}.mime").takeIf { it.exists() }?.readText() ?: "image/png"
        return "data:$mime;base64," + Base64.encodeToString(entry.readBytes(), Base64.NO_WRAP)
    }

    fun contentUri(context: Context, repository: AuthRepository, path: String?): Uri? {
        if (path.isNullOrBlank()) return null
        val entry = runCatching { ensure(context, repository, path) }.getOrNull() ?: return null
        return Uri.Builder().scheme("content").authority(AUTHORITY).appendPath(entry.name).build()
    }

    fun file(context: Context, key: String): File? {
        if (!key.matches(Regex("^[a-f0-9]{64}$"))) return null
        return File(directory(context), key).takeIf { it.isFile && it.length() > 0 }
    }

    fun mime(context: Context, key: String): String =
        File(directory(context), "$key.mime").takeIf { it.isFile }?.readText()?.trim().orEmpty().ifBlank { "image/png" }

    fun clear(context: Context) {
        directory(context).listFiles()?.forEach { it.delete() }
    }

    private fun ensure(context: Context, repository: AuthRepository, path: String): File {
        require(path.startsWith("/images/")) { "Only Xivi artwork can be cached" }
        val key = MessageDigest.getInstance("SHA-256")
            .digest("${repository.server()}|$path".toByteArray())
            .joinToString("") { "%02x".format(it) }
        val target = File(directory(context), key)
        if (target.isFile && target.length() > 0) return target
		val (bytes, mime) = repository.download(path)
		target.writeBytes(bytes)
		File(directory(context), "$key.mime").writeText(mime)
        return target
    }
}
