import java.util.Properties

plugins {
    id("com.android.application")
    id("kotlin-android")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

// release 签名：读 android/key.properties（存在时用 release keystore，否则回退 debug）。
// key.properties 与 *.jks 不提交 git（见 .gitignore）；CI 由 ANDROID_* secrets 注入。
// 本地首次配置见 mobile/android/key.properties.example。
val keystoreProperties = Properties()
val keystorePropertiesFile = rootProject.file("key.properties")
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(keystorePropertiesFile.inputStream())
}

android {
    namespace = "com.hxpt.ptpatronus"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = JavaVersion.VERSION_17.toString()
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "com.hxpt.ptpatronus"
        // You can update the following values to match your application needs.
        // For more information, visit: https://flutter.dev/to/review-gradle-config.
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    signingConfigs {
        create("release") {
            keystoreProperties["keyAlias"]?.let { keyAlias = it as String }
            keystoreProperties["keyPassword"]?.let { keyPassword = it as String }
            keystoreProperties["storeFile"]?.let { storeFile = file(it as String) }
            keystoreProperties["storePassword"]?.let { storePassword = it as String }
        }
    }

    buildTypes {
        release {
            // 签名配置缺失直接 fail：静默回退 debug 签名的 APK 一旦流出，
            // 用户装了之后正式签名版将无法覆盖安装（安全审计 P1-2）。
            // 本地打包：app/key.properties + keystore；CI：ANDROID_KEYSTORE_BASE64。
            if (!keystorePropertiesFile.exists()) {
                throw GradleException(
                    "Release build requires signing config: put key.properties (keyAlias/keyPassword/storeFile/storePassword) " +
                    "in mobile/android/app/, or set ANDROID_KEYSTORE_BASE64 in CI. " +
                    "Refusing to fall back to debug signing."
                )
            }
            signingConfig = signingConfigs.getByName("release")
            // R8 收缩：减包体 + 收窄逆向面（JWT/密钥处理逻辑不进 dex 明文）。
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }
}

flutter {
    source = "../.."
}
