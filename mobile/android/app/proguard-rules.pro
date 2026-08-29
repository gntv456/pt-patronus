# Flutter 默认规则已由 proguard-android-optimize.txt 提供；
# Flutter engine 与 dart2js 产物不需要额外 keep（flutter 工具链自带规则）。
# 仅补充原生插件反射访问的类。

# flutter_secure_storage（AndroidKeystore 反射）
-keep class com.it_nomads.fluttersecurestorage.** { *; }

# media_kit / video_player 原生桥
-keep class com.ryanheike.audiotrack.** { *; }
-dontwarn com.ryanheike.audiotrack.**
