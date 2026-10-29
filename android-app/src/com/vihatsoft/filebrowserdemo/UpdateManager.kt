package com.vihatsoft.filebrowserdemo

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.Arrays

object UpdateManager {
    private const val API = "https://api.github.com/repos/viratsoft/filebrowser/releases/latest"
    private const val ASSET = "filebrowser-server-app-arm64.apk"
    data class Release(val tag: String, val page: String, val apkUrl: String?, val digest: String?)

    fun check(): Release {
        val connection = URL(API).openConnection() as HttpURLConnection
        connection.connectTimeout = 10000
        connection.readTimeout = 10000
        connection.setRequestProperty("Accept", "application/vnd.github+json")
        connection.setRequestProperty("User-Agent", "Filebrowser-Android-Test")
        try {
            if (connection.responseCode != 200) throw IllegalStateException("GitHub returned HTTP " + connection.responseCode)
            val json = JSONObject(connection.inputStream.bufferedReader().use { it.readText() })
            val tag = json.getString("tag_name")
            val assets = json.getJSONArray("assets")
            var apkUrl: String? = null
            var digest: String? = null
            for (i in 0 until assets.length()) {
                val asset = assets.getJSONObject(i)
                if (asset.getString("name") == ASSET) {
                    apkUrl = asset.getString("browser_download_url")
                    digest = asset.optString("digest").ifBlank { null }
                    break
                }
            }
            return Release(tag, json.getString("html_url"), apkUrl, digest)
        } finally { connection.disconnect() }
    }

    fun newer(latest: String, current: String): Boolean {
        val a = parse(latest) ?: return false
        val b = parse(current) ?: return false
        for (i in 0..2) {
            if (a[i] != b[i]) return a[i] > b[i]
        }
        return false
    }
    private fun parse(value: String): List<Int>? {
        val parts = value.removePrefix("v").split('.')
        if (parts.size != 3) return null
        return try { parts.map { it.toInt() } } catch (_: NumberFormatException) { null }
    }

    fun downloadAndVerify(context: Context, release: Release): File {
        val url = release.apkUrl ?: throw IllegalStateException("Android APK is not published for " + release.tag)
        val temp = File(context.cacheDir, "filebrowser-update.tmp")
        val target = UpdateProvider.file(context)
        temp.delete()
        val connection = URL(url).openConnection() as HttpURLConnection
        connection.connectTimeout = 15000
        connection.readTimeout = 30000
        connection.setRequestProperty("User-Agent", "Filebrowser-Android-Test")
        val sha = MessageDigest.getInstance("SHA-256")
        try {
            if (connection.responseCode != 200) throw IllegalStateException("Download returned HTTP " + connection.responseCode)
            var count = 0L
            connection.inputStream.use { input ->
                temp.outputStream().use { output ->
                    val buffer = ByteArray(64 * 1024)
                    while (true) {
                        val n = input.read(buffer)
                        if (n < 0) break
                        count += n
                        if (count > 200L * 1024 * 1024) throw IllegalStateException("Update exceeds 200 MB")
                        output.write(buffer, 0, n)
                        sha.update(buffer, 0, n)
                    }
                }
            }
            val expected = release.digest?.removePrefix("sha256:")
            val actual = sha.digest().joinToString("") { "%02x".format(it.toInt() and 0xff) }
            if (expected != null && !actual.equals(expected, true)) throw IllegalStateException("Update checksum mismatch")
            verifyPackage(context, temp, release.tag)
            target.delete()
            if (!temp.renameTo(target)) throw IllegalStateException("Cannot save update")
            return target
        } catch (e: Exception) {
            temp.delete()
            throw e
        } finally { connection.disconnect() }
    }

    private fun verifyPackage(context: Context, apk: File, expectedTag: String) {
        val pm = context.packageManager
        val flags = if (Build.VERSION.SDK_INT >= 28) PackageManager.GET_SIGNING_CERTIFICATES else PackageManager.GET_SIGNATURES
        val candidate = pm.getPackageArchiveInfo(apk.absolutePath, flags) ?: throw IllegalStateException("Invalid Android APK")
        val installed = pm.getPackageInfo(context.packageName, flags)
        if (candidate.packageName != context.packageName) throw IllegalStateException("Update has a different package ID")
        if (candidate.versionName != expectedTag.removePrefix("v")) throw IllegalStateException("Update version does not match release")
        if (candidate.versionCode <= installed.versionCode) throw IllegalStateException("Update version code is not newer")
        val a = if (Build.VERSION.SDK_INT >= 28) candidate.signingInfo?.apkContentsSigners?.firstOrNull() else candidate.signatures?.firstOrNull()
        val b = if (Build.VERSION.SDK_INT >= 28) installed.signingInfo?.apkContentsSigners?.firstOrNull() else installed.signatures?.firstOrNull()
        if (a == null || b == null || !Arrays.equals(a.toByteArray(), b.toByteArray())) throw IllegalStateException("Update signing key does not match")
    }

    fun clearInstalledDownload(context: Context) {
        val file = UpdateProvider.file(context)
        if (!file.exists()) return
        try {
            val candidate = context.packageManager.getPackageArchiveInfo(file.absolutePath, 0)
            val installed = context.packageManager.getPackageInfo(context.packageName, 0)
            if (candidate == null || candidate.versionCode <= installed.versionCode) file.delete()
        } catch (_: Exception) { file.delete() }
    }
}
