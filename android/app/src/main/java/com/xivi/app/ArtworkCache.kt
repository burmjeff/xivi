package com.xivi.app

import android.content.Context
import android.net.Uri
import android.util.Base64
import java.io.File
import java.security.MessageDigest

object ArtworkCache {
    const val AUTHORITY = "com.xivi.app.artwork"
    private val locks = Array(32) { Any() }

    private fun directory(context: Context) = File(context.cacheDir, "artwork").apply { mkdirs() }

    fun dataUrl(context: Context, repository: NativeApi, path: String): String {
        val entry = ensure(context, repository, path)
        val mime = File(entry.parentFile, "${entry.name}.mime").takeIf { it.exists() }?.readText() ?: "image/png"
        return "data:$mime;base64," + Base64.encodeToString(entry.readBytes(), Base64.NO_WRAP)
    }

    fun contentUri(context: Context, repository: NativeApi, path: String?): Uri? {
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

    private fun ensure(context: Context, repository: NativeApi, path: String): File {
        require(path.startsWith("/images/")) { "Only Xivi artwork can be cached" }
        val key = MessageDigest.getInstance("SHA-256")
            .digest("${repository.cacheNamespace()}|$path".toByteArray())
            .joinToString("") { "%02x".format(it) }
        val target = File(directory(context), key)
        if (target.isFile && target.length() > 0) return target
        return synchronized(locks[Math.floorMod(key.hashCode(), locks.size)]) {
            if (target.isFile && target.length() > 0) return@synchronized target
            val (bytes, mime) = repository.download(path)
            require(bytes.size <= 4 * 1024 * 1024) { "Artwork exceeds cache limit" }
            val temporary = File(directory(context), "$key.tmp")
            temporary.writeBytes(bytes)
            check(temporary.renameTo(target)) { "Artwork cache could not be saved" }
            File(directory(context), "$key.mime").writeText(mime)
            val entries = directory(context).listFiles()?.filter { it.name.matches(Regex("^[a-f0-9]{64}$")) }?.sortedBy { it.lastModified() }.orEmpty()
            var size = entries.sumOf { it.length() }
            for (entry in entries) {
                if (size <= 64 * 1024 * 1024) break
                if (entry != target) { size -= entry.length(); entry.delete(); File(entry.parentFile, "${entry.name}.mime").delete() }
            }
            target
        }
    }
}
