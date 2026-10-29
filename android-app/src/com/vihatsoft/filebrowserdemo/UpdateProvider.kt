package com.vihatsoft.filebrowserdemo

import android.content.ContentProvider
import android.content.ContentValues
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.provider.OpenableColumns
import java.io.File
import java.io.FileNotFoundException

class UpdateProvider : ContentProvider() {
    companion object {
        const val AUTHORITY = "com.vihatsoft.filebrowserdemo.update"
        val URI: Uri = Uri.parse("content://" + AUTHORITY + "/filebrowser-update.apk")
        fun file(context: android.content.Context): File = File(context.cacheDir, "filebrowser-update.apk")
    }
    override fun onCreate(): Boolean = true
    override fun getType(uri: Uri): String? = if (uri == URI) "application/vnd.android.package-archive" else null
    override fun openFile(uri: Uri, mode: String): ParcelFileDescriptor {
        if (uri != URI || mode != "r") throw FileNotFoundException("Invalid update request")
        return ParcelFileDescriptor.open(file(context ?: throw FileNotFoundException()), ParcelFileDescriptor.MODE_READ_ONLY)
    }
    override fun query(uri: Uri, projection: Array<out String>?, selection: String?, selectionArgs: Array<out String>?, sortOrder: String?): Cursor? {
        if (uri != URI) return null
        val apk = file(context ?: return null)
        val columns = projection ?: arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE)
        val cursor = MatrixCursor(columns)
        cursor.addRow(columns.map { when (it) {
            OpenableColumns.DISPLAY_NAME -> "filebrowser-update.apk"
            OpenableColumns.SIZE -> apk.length()
            else -> null
        } }.toTypedArray())
        return cursor
    }
    override fun insert(uri: Uri, values: ContentValues?): Uri? = null
    override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?): Int = 0
    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = 0
}
