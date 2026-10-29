package com.vihatsoft.filebrowserdemo

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Environment
import android.os.IBinder
import android.util.Log
import java.io.File
import java.util.ArrayDeque
import java.util.concurrent.TimeUnit

class ServerService : Service() {
    companion object {
        const val START = "com.vihatsoft.filebrowserdemo.START"
        const val STOP = "com.vihatsoft.filebrowserdemo.STOP"
        const val CHANGE_PASSWORD = "com.vihatsoft.filebrowserdemo.CHANGE_PASSWORD"
        const val EXTRA_PASSWORD = "newPassword"
        private const val CHANNEL = "server"
        @Volatile var running = false
        @Volatile var changing = false
        @Volatile var error: String? = null
        private val logLines = ArrayDeque<String>()
        private fun log(message: String) = synchronized(logLines) {
            logLines.addLast(message)
            while (logLines.size > 250) logLines.removeFirst()
        }
        fun logs(): String = synchronized(logLines) { logLines.joinToString("\n", postfix = if (logLines.isEmpty()) "" else "\n") }
        fun sharedRoot(context: Context): File = File(context.getExternalFilesDir(null) ?: context.filesDir, "shared")
    }

    @Volatile private var process: Process? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == STOP) {
            stopServer(true)
            stopSelf()
            return START_NOT_STICKY
        }
        val change = intent?.action == CHANGE_PASSWORD
        if (!change && process?.isAlive == true) return START_STICKY
        showNotification()
        if (change) {
            val password = intent?.getStringExtra(EXTRA_PASSWORD) ?: ""
            if (password.codePointCount(0, password.length) < 4) return fail("Password must have at least 4 characters")
            return changePasswordAndStart(password)
        }
        return startServer(null)
    }

    private fun showNotification() {
        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        manager.createNotificationChannel(NotificationChannel(CHANNEL, "Filebrowser server", NotificationManager.IMPORTANCE_LOW))
        val stopIntent = Intent(this, ServerService::class.java).setAction(STOP)
        val stopAction = PendingIntent.getService(this, 1, stopIntent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val openAction = PendingIntent.getActivity(this, 2, Intent(this, MainActivity::class.java), PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val notification = Notification.Builder(this, CHANNEL)
            .setSmallIcon(android.R.drawable.stat_sys_upload)
            .setContentTitle("Filebrowser running")
            .setContentText("Port 8080 · Tap to view address")
            .setContentIntent(openAction)
            .addAction(Notification.Action.Builder(android.R.drawable.ic_media_pause, "Stop", stopAction).build())
            .build()
        startForeground(1, notification)
    }

    private fun changePasswordAndStart(newPassword: String): Int {
        changing = true
        error = null
        val old = process
        process = null
        running = false
        if (old != null) {
            old.destroy()
            if (!old.waitFor(3, TimeUnit.SECONDS)) {
                old.destroyForcibly()
                old.waitFor()
            }
        }
        val executable = applicationInfo.nativeLibraryDir + "/libfilebrowser.so"
        val db = File(filesDir, "filebrowser.db")
        val initialHash: String?
        try {
            if (db.exists()) {
                val settings = runCommand(executable, listOf("--database", db.absolutePath, "config", "set", "--minimumPasswordLength", "4"))
                if (settings.first != 0) return fail("Could not set the 4-character password policy")
                val result = runCommand(executable, listOf("--database", db.absolutePath, "users", "update", "admin", "--password", newPassword))
                if (result.first != 0) return fail("Filebrowser rejected the password. Choose a different 4+ character password")
                initialHash = null
            } else {
                val result = runCommand(executable, listOf("hash", newPassword))
                val hash = result.second.lines().lastOrNull { it.startsWith("$" + "2") }
                if (result.first != 0 || hash == null) return fail("Could not prepare password")
                initialHash = hash
            }
            getSharedPreferences("server", MODE_PRIVATE).edit().putString("password", newPassword).commit()
            log("Admin password saved")
            val result = startServer(initialHash)
            changing = false
            return result
        } catch (e: Exception) { return fail("Password update failed: " + e.message) }
    }

    private fun runCommand(executable: String, args: List<String>): Pair<Int, String> {
        val command = ProcessBuilder(listOf(executable) + args).directory(filesDir).redirectErrorStream(true)
        command.environment()["HOME"] = filesDir.absolutePath
        val child = command.start()
        val output = child.inputStream.bufferedReader().use { it.readText() }
        return child.waitFor() to output.trim()
    }

    private fun startServer(initialHash: String?): Int {
        val selected = getSharedPreferences("server", MODE_PRIVATE).getString("selectedPath", null)
        val root = if (selected == null) sharedRoot(this) else File(selected)
        val db = File(filesDir, "filebrowser.db")
        if (selected == null && !root.exists() && !root.mkdirs()) return fail("Cannot create shared folder")
        if (selected != null && Build.VERSION.SDK_INT >= 30 && !Environment.isExternalStorageManager()) return fail("Grant all files access for selected folder")
        if (!root.isDirectory || !root.canRead() || !root.canWrite()) return fail("Selected folder is not readable and writable: " + root)
        if (!db.exists() && initialHash == null) return fail("Set a custom password before first start")
        return try {
            log("Starting Filebrowser · root: " + root.absolutePath)
            val executable = applicationInfo.nativeLibraryDir + "/libfilebrowser.so"
            val args = mutableListOf(executable, "--address", "0.0.0.0", "--port", "8080", "--root", root.absolutePath, "--database", db.absolutePath)
            if (initialHash != null) args.addAll(listOf("--password", initialHash))
            val builder = ProcessBuilder(args).directory(filesDir).redirectErrorStream(true)
            builder.environment()["HOME"] = filesDir.absolutePath
            val launched = builder.start()
            process = launched
            running = true
            error = null
            Thread({ readOutput(launched) }, "filebrowser-output").start()
            START_STICKY
        } catch (e: Exception) { fail(e.toString()) }
    }

    private fun readOutput(launched: Process) {
        try {
            launched.inputStream.bufferedReader().use { reader ->
                reader.forEachLine { line ->
                    Log.i("Filebrowser", line)
                    log(line)
                    val marker = "randomly generated password: "
                    val at = line.indexOf(marker)
                    if (at >= 0) getSharedPreferences("server", MODE_PRIVATE).edit().putString("password", line.substring(at + marker.length).trim()).apply()
                }
            }
            val code = launched.waitFor()
            if (process === launched) {
                running = false
                error = "Server exited (code " + code + ")"
                log(error ?: "Server exited")
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
            }
        } catch (e: Exception) { if (process === launched) fail(e.toString()) }
    }
    private fun fail(message: String): Int {
        running = false
        changing = false
        error = message
        log("Error: " + message)
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
        return START_NOT_STICKY
    }
    private fun stopServer(clearError: Boolean) {
        val old = process
        process = null
        running = false
        changing = false
        if (clearError) error = null
        old?.destroy()
        if (clearError) log("Server stopped")
        stopForeground(STOP_FOREGROUND_REMOVE)
    }
    override fun onDestroy() { stopServer(false); super.onDestroy() }
    override fun onBind(intent: Intent?): IBinder? = null
}
