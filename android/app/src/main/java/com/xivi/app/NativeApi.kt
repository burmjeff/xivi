package com.xivi.app

import okhttp3.OkHttpClient

/** Shared native edge; presentation and credential policy belong to each surface. */
interface NativeApi {
    fun server(): String?
    fun absoluteUrl(path: String): String
    fun mediaClient(): OkHttpClient
    fun cacheNamespace(): String = server().orEmpty()
    fun request(path: String, method: String, suppliedHeaders: Map<String, String> = emptyMap(), suppliedBody: String? = null): NativeHttpResponse
    fun download(path: String): Pair<ByteArray, String>
}
