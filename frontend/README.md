# BharatChat Flutter Client

Cross-platform Flutter client for BharatChat. Platform runners are checked in
for Android, iOS, web, Windows, macOS, and Linux.

## Develop

```bash
flutter pub get
flutter run \
  --dart-define=BASE_URL=http://10.0.2.2:8080/api/v1 \
  --dart-define=ALLOW_PLAINTEXT_MESSAGING=true
```

Use `http://localhost:8080/api/v1` for web and desktop clients running on the
same host as the backend.

`ALLOW_PLAINTEXT_MESSAGING` is for local UI development only. Release builds
fail at startup if it is enabled.

## Verify

```bash
dart format --output=none --set-exit-if-changed lib test
flutter analyze
flutter test
flutter build web --release \
  --dart-define=BASE_URL=https://api.example.invalid/api/v1
```

Android release builds require a private upload keystore. Copy
`android/key.properties.example` to the ignored `android/key.properties` file
and replace every placeholder. Never commit the keystore or real passwords.
