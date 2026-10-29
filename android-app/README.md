# Android Filebrowser test app

This Kotlin wrapper runs the repository's Filebrowser server on Android ARM64. Its release APK includes the server binary and original web UI. The app checks `viratsoft/filebrowser` releases for a newer server version. An update is offered only when that release contains `filebrowser-server-app-arm64.apk`, signed with the same key and carrying a higher Android version code. Android replaces the installed package; the app deletes its downloaded update after installation.

The release job in `.github/workflows/ci.yaml` builds the matching server binary, signs the wrapper, and attaches that APK to the Filebrowser release. Before using it, set the repository secret `ANDROID_KEYSTORE_BASE64` to the base64 encoding of the existing `build/debug.keystore`. Retain that key for every later release. The test wrapper currently uses the Android debug key alias and passwords; use a dedicated release key before distributing beyond the test device.

To build locally on Windows, provide Android SDK platform 35, build tools 35.0.0 and 36.0.0, JDK 17+, Kotlin compiler 2.1.20 at `tools/kotlinc`, an ARM64 Filebrowser executable at `lib/arm64-v8a/libfilebrowser.so`, then run `build.cmd`.
