package com.vihatsoft.filebrowserdemo

import android.app.Activity
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.os.Handler
import android.os.Looper
import android.provider.DocumentsContract
import android.provider.Settings
import android.text.InputType
import android.text.method.PasswordTransformationMethod
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import java.io.File
import java.net.Inet4Address
import java.net.NetworkInterface
import java.util.Collections

class MainActivity : Activity() {
    private val pickFolder = 41
    private val handler = Handler(Looper.getMainLooper())
    private lateinit var status: TextView
    private lateinit var updateStatus: TextView
    private lateinit var updateButton: Button
    private var availableUpdate: UpdateManager.Release? = null
    private var pendingInstall = false
    private lateinit var folder: TextView
    private lateinit var password: TextView
    private lateinit var logs: TextView
    private lateinit var logScroll: ScrollView
    private lateinit var open: Button
    private val refresh = object : Runnable {
        override fun run() {
            update()
            handler.postDelayed(this, 1000)
        }
    }

    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(32, 40, 32, 32)
        }
        addText(box, "Filebrowser Test", 26)
        addText(box, "viratsoft/filebrowser " + currentVersion() + " · Same Wi-Fi required for LAN access", 15)
        UpdateManager.clearInstalledDownload(this)
        updateStatus = addText(box, "Current Filebrowser: " + currentVersion() + "\nLatest: Not checked", 15)
        button(box, "Check Filebrowser update") { checkUpdates() }
        updateButton = button(box, "Update Filebrowser") { updateRelease() }
        updateButton.isEnabled = false
        status = addText(box, "", 17)
        folder = addText(box, "", 15)
        val newPassword = EditText(this).apply {
            hint = "New admin password (4+ characters)"
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD
            transformationMethod = PasswordTransformationMethod.getInstance()
            isSingleLine = true
        }
        box.addView(newPassword)
        val confirmPassword = EditText(this).apply {
            hint = "Confirm new password"
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD
            transformationMethod = PasswordTransformationMethod.getInstance()
            isSingleLine = true
        }
        box.addView(confirmPassword)
        button(box, "Save password and start") {
            val chosen = newPassword.text.toString()
            if (chosen.codePointCount(0, chosen.length) < 4) {
                Toast.makeText(this, "Use at least 4 characters", Toast.LENGTH_LONG).show()
            } else if (chosen != confirmPassword.text.toString()) {
                Toast.makeText(this, "Passwords do not match", Toast.LENGTH_LONG).show()
            } else if (selectedPath() != null && Build.VERSION.SDK_INT >= 30 && !Environment.isExternalStorageManager()) {
                Toast.makeText(this, "Grant all files access first", Toast.LENGTH_LONG).show()
                requestAllFilesAccess()
            } else {
                val i = Intent(this, ServerService::class.java).setAction(ServerService.CHANGE_PASSWORD)
                    .putExtra(ServerService.EXTRA_PASSWORD, chosen)
                startForegroundService(i)
                newPassword.text.clear()
                confirmPassword.text.clear()
                update()
            }
        }
        button(box, "Choose phone folder") { chooseFolder() }
        button(box, "Use app private folder") {
            stopIfRunning()
            prefs().edit().remove("selectedPath").remove("selectedUri").apply()
            update()
        }
        button(box, "Grant all files access") { requestAllFilesAccess() }
        button(box, "Start server") {
            if (!File(filesDir, "filebrowser.db").exists()) {
                Toast.makeText(this, "Set a custom password first", Toast.LENGTH_LONG).show()
            } else if (selectedPath() != null && Build.VERSION.SDK_INT >= 30 && !Environment.isExternalStorageManager()) {
                Toast.makeText(this, "Grant all files access, then tap Start", Toast.LENGTH_LONG).show()
                requestAllFilesAccess()
            } else {
                startForegroundService(Intent(this, ServerService::class.java).setAction(ServerService.START))
                update()
            }
        }
        button(box, "Stop server") { stopIfRunning() }
        open = button(box, "Open Filebrowser on this phone") {
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("http://127.0.0.1:8080/")))
        }
        button(box, "Copy LAN address") { copy("LAN address", "http://" + lanIp() + ":8080/") }
        addText(box, "Username: admin", 16)
        button(box, "Copy username") { copy("Username", "admin") }
        password = addText(box, "", 16)
        button(box, "Copy password") { copy("Password", prefs().getString("password", "") ?: "") }
        addText(box, "Server logs", 19)
        button(box, "Copy logs") { copy("Server logs", ServerService.logs()) }
        logScroll = ScrollView(this).apply { setBackgroundColor(0xffeeeeee.toInt()) }
        logs = TextView(this).apply {
            textSize = 12f
            setTextIsSelectable(true)
            setPadding(16, 16, 16, 16)
        }
        logScroll.addView(logs)
        box.addView(logScroll, LinearLayout.LayoutParams(-1, dp(260)))
        setContentView(ScrollView(this).apply { addView(box) })
    }

    private fun addText(box: LinearLayout, value: String, size: Int): TextView = TextView(this).also {
        it.text = value
        it.textSize = size.toFloat()
        it.setTextIsSelectable(true)
        it.setPadding(0, 8, 0, 8)
        box.addView(it)
    }
    private fun button(box: LinearLayout, label: String, action: () -> Unit): Button = Button(this).also {
        it.text = label
        it.setOnClickListener { action() }
        box.addView(it)
    }
    private fun dp(value: Int) = (value * resources.displayMetrics.density + 0.5f).toInt()
    private fun currentVersion(): String = "v" + (packageManager.getPackageInfo(packageName, 0).versionName ?: "0.1.35")
    private fun checkUpdates() {
        updateButton.isEnabled = false
        updateStatus.text = "Current Filebrowser: " + currentVersion() + "\nChecking GitHub..."
        Thread {
            try {
                val release = UpdateManager.check()
                runOnUiThread {
                    val newer = UpdateManager.newer(release.tag, currentVersion())
                    availableUpdate = if (newer && release.apkUrl != null) release else null
                    updateButton.isEnabled = availableUpdate != null
                    val result = when {
                        !newer -> "Already up to date"
                        release.apkUrl == null -> "Update found, but Android build is not published yet"
                        else -> "Update found"
                    }
                    updateStatus.text = "Current Filebrowser: " + currentVersion() + "\nLatest Filebrowser: " + release.tag + "\n" + result
                }
            } catch (e: Exception) {
                runOnUiThread { updateStatus.text = "Current Filebrowser: " + currentVersion() + "\nUpdate check failed: " + e.message }
            }
        }.start()
    }
    private fun updateRelease() {
        val release = availableUpdate ?: return
        updateButton.isEnabled = false
        updateStatus.text = "Current Filebrowser: " + currentVersion() + "\nLatest Filebrowser: " + release.tag + "\nDownloading Android build..."
        Thread {
            try {
                UpdateManager.downloadAndVerify(this, release)
                runOnUiThread {
                    updateStatus.text = "Current Filebrowser: " + currentVersion() + "\nLatest Filebrowser: " + release.tag + "\nConfirm replacement in Android installer"
                    installDownloaded()
                }
            } catch (e: Exception) {
                runOnUiThread {
                    updateButton.isEnabled = true
                    updateStatus.text = "Current Filebrowser: " + currentVersion() + "\nLatest Filebrowser: " + release.tag + "\nUpdate failed: " + e.message
                }
            }
        }.start()
    }
    private fun installDownloaded() {
        if (!UpdateProvider.file(this).exists()) return
        if (!packageManager.canRequestPackageInstalls()) {
            pendingInstall = true
            startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + packageName)))
            return
        }
        pendingInstall = false
        val intent = Intent(Intent.ACTION_INSTALL_PACKAGE)
            .setDataAndType(UpdateProvider.URI, "application/vnd.android.package-archive")
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        startActivity(intent)
    }
    private fun prefs() = getSharedPreferences("server", MODE_PRIVATE)
    private fun selectedPath(): String? = prefs().getString("selectedPath", null)
    private fun copy(label: String, value: String) {
        if (value.isEmpty()) {
            Toast.makeText(this, "Nothing to copy yet", Toast.LENGTH_SHORT).show()
            return
        }
        (getSystemService(CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText(label, value))
        Toast.makeText(this, label + " copied", Toast.LENGTH_SHORT).show()
    }
    private fun stopIfRunning() {
        startService(Intent(this, ServerService::class.java).setAction(ServerService.STOP))
        update()
    }
    private fun chooseFolder() {
        if (Build.VERSION.SDK_INT < 30) {
            Toast.makeText(this, "Folder selection needs Android 11 or newer", Toast.LENGTH_LONG).show()
            return
        }
        val intent = Intent(Intent.ACTION_OPEN_DOCUMENT_TREE).addFlags(
            Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION or Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION
        )
        startActivityForResult(intent, pickFolder)
    }
    @Deprecated("Folder picker callback")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        val uri = data?.data ?: return
        if (requestCode != pickFolder || resultCode != RESULT_OK) return
        if (uri.authority != "com.android.externalstorage.documents") {
            Toast.makeText(this, "Choose a folder in this phone's local storage", Toast.LENGTH_LONG).show()
            return
        }
        try {
            val id = DocumentsContract.getTreeDocumentId(uri)
            val colon = id.indexOf(':')
            require(colon >= 0) { "Unsupported folder" }
            val volume = id.substring(0, colon)
            val relative = id.substring(colon + 1)
            val base = if (volume.equals("primary", true)) File("/storage/emulated/0") else File("/storage", volume)
            val root = File(base, relative).canonicalFile
            val basePath = base.canonicalPath
            require(root.path == basePath || root.path.startsWith(basePath + File.separator)) { "Invalid folder path" }
            val flags = data.flags and (Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
            if (flags != 0) contentResolver.takePersistableUriPermission(uri, flags)
            stopIfRunning()
            prefs().edit().putString("selectedPath", root.path).putString("selectedUri", uri.toString()).apply()
            update()
            if (!Environment.isExternalStorageManager()) requestAllFilesAccess()
        } catch (e: Exception) {
            Toast.makeText(this, "Cannot use folder: " + e.message, Toast.LENGTH_LONG).show()
        }
    }
    private fun requestAllFilesAccess() {
        if (Build.VERSION.SDK_INT < 30 || Environment.isExternalStorageManager()) {
            Toast.makeText(this, "Storage access already available", Toast.LENGTH_SHORT).show()
            return
        }
        val intent = Intent(Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION, Uri.parse("package:" + packageName))
        try { startActivity(intent) }
        catch (_: Exception) { startActivity(Intent(Settings.ACTION_MANAGE_ALL_FILES_ACCESS_PERMISSION)) }
    }
    override fun onResume() {
        super.onResume()
        handler.post(refresh)
        if (pendingInstall && packageManager.canRequestPackageInstalls()) installDownloaded()
    }
    override fun onPause() { handler.removeCallbacks(refresh); super.onPause() }
    private fun update() {
        val state = if (ServerService.changing) "Saving password" else if (ServerService.running) "Running" else if (ServerService.error == null) "Stopped" else "Error: " + ServerService.error
        status.text = "Status: " + state + "\nLAN: http://" + lanIp() + ":8080/"
        val selected = selectedPath()
        val root = if (selected == null) ServerService.sharedRoot(this) else File(selected)
        val access = if (selected == null) "App private folder (no permission needed)" else if (Build.VERSION.SDK_INT >= 30 && Environment.isExternalStorageManager()) "Phone folder · access granted" else "Phone folder · grant all files access"
        folder.text = "Shared folder: " + root.absolutePath + "\n" + access
        password.text = "Password: " + prefs().getString("password", "Waiting for first start...")
        open.isEnabled = ServerService.running
        val next = ServerService.logs()
        if (logs.text.toString() != next) {
            val atBottom = logScroll.scrollY + logScroll.height >= logs.height - dp(24)
            logs.text = next
            if (atBottom) logScroll.post { logScroll.fullScroll(View.FOCUS_DOWN) }
        }
    }
    private fun lanIp(): String {
        var fallback: String? = null
        try {
            for (network in Collections.list(NetworkInterface.getNetworkInterfaces())) {
                if (!network.isUp || network.isLoopback) continue
                for (address in Collections.list(network.inetAddresses)) {
                    if (address is Inet4Address && address.isSiteLocalAddress) {
                        if (network.name.startsWith("wlan") || network.name.startsWith("wifi")) return address.hostAddress ?: "Wi-Fi IP unavailable"
                        if (fallback == null) fallback = address.hostAddress
                    }
                }
            }
        } catch (_: Exception) { }
        return fallback ?: "Wi-Fi IP unavailable"
    }
}
