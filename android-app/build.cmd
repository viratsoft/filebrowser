@echo off
setlocal
set "SDK=%ANDROID_SDK_ROOT%"
if not defined ANDROID_SDK_ROOT set "SDK=%ANDROID_HOME%"
if not defined ANDROID_SDK_ROOT if not defined ANDROID_HOME set "SDK=%LOCALAPPDATA%\Android\Sdk"
if not exist "%SDK%\platforms\android-35\android.jar" set "SDK=C:\Users\Admin\AppData\Local\Android\Sdk"
set "BT=%SDK%\build-tools\35.0.0"
set "DX=%SDK%\build-tools\36.0.0"
if not defined JAVA_HOME set "JAVA_HOME=C:\Program Files\Android\Android Studio\jbr"
set "JDK=%JAVA_HOME%\bin"
set "ROOT=%~dp0"
set "KOTLIN=%ROOT%tools\kotlinc\bin\kotlinc.bat"
if not exist "%ROOT%lib\arm64-v8a\libfilebrowser.so" (echo Missing Android ARM64 Filebrowser binary & exit /b 1)
if not exist "%BT%\aapt2.exe" (echo Android SDK build-tools 35.0.0 required & exit /b 1)
if not exist "%DX%\lib\d8.jar" (echo Android SDK build-tools 36.0.0 required for Kotlin & exit /b 1)
if not exist "%ROOT%build" mkdir "%ROOT%build"
"%BT%\aapt2.exe" link -o "%ROOT%build\base.apk" -I "%SDK%\platforms\android-35\android.jar" --manifest "%ROOT%AndroidManifest.xml" --min-sdk-version 26 --target-sdk-version 32 || exit /b 1
call "%KOTLIN%" -jvm-target 1.8 -include-runtime -classpath "%SDK%\platforms\android-35\android.jar" -d "%ROOT%build\app.jar" "%ROOT%src\com\vihatsoft\filebrowserdemo\MainActivity.kt" "%ROOT%src\com\vihatsoft\filebrowserdemo\ServerService.kt" "%ROOT%src\com\vihatsoft\filebrowserdemo\UpdateManager.kt" "%ROOT%src\com\vihatsoft\filebrowserdemo\UpdateProvider.kt" || exit /b 1
if exist "%ROOT%build\classes.dex" del /q "%ROOT%build\classes.dex"
"%JDK%\java.exe" -Xmx1024m -cp "%DX%\lib\d8.jar" com.android.tools.r8.D8 --min-api 26 --lib "%SDK%\platforms\android-35\android.jar" --output "%ROOT%build" "%ROOT%build\app.jar" || exit /b 1
copy /y "%ROOT%build\base.apk" "%ROOT%build\unpacked.apk" >nul
"%JDK%\jar.exe" uf "%ROOT%build\unpacked.apk" -C "%ROOT%build" classes.dex -C "%ROOT%." lib || exit /b 1
"%BT%\zipalign.exe" -f 4 "%ROOT%build\unpacked.apk" "%ROOT%build\aligned.apk" || exit /b 1
if not exist "%ROOT%build\debug.keystore" "%JDK%\keytool.exe" -genkeypair -keystore "%ROOT%build\debug.keystore" -storepass android -keypass android -alias androiddebugkey -dname "CN=Android Debug,O=Android,C=US" -keyalg RSA -keysize 2048 -validity 10000 || exit /b 1
call "%BT%\apksigner.bat" sign --ks "%ROOT%build\debug.keystore" --ks-key-alias androiddebugkey --ks-pass pass:android --key-pass pass:android --out "%ROOT%filebrowser-test.apk" "%ROOT%build\aligned.apk" || exit /b 1
call "%BT%\apksigner.bat" verify "%ROOT%filebrowser-test.apk" || exit /b 1
echo Built: %ROOT%filebrowser-test.apk
