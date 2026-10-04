import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:hive_flutter/hive_flutter.dart';

class HiveBoxes {
  static const cachedUserBox = 'cached_user_box';
  static const messageOutboxBox = 'message_outbox_box';
  static const _outboxKeyName = 'bharatchat_outbox_key_v1';

  static Future<void> init() async {
    await Hive.initFlutter();
    await Hive.openBox(cachedUserBox);

    const secureStorage = FlutterSecureStorage(
      aOptions: AndroidOptions(encryptedSharedPreferences: true),
    );
    var encodedKey = await secureStorage.read(key: _outboxKeyName);
    if (encodedKey == null) {
      encodedKey = base64UrlEncode(Hive.generateSecureKey());
      await secureStorage.write(key: _outboxKeyName, value: encodedKey);
    }
    final key = base64Url.decode(encodedKey);
    if (key.length != 32) {
      throw StateError('Invalid encrypted outbox key');
    }
    await Hive.openBox(messageOutboxBox, encryptionCipher: HiveAesCipher(key));
  }

  static Box get cachedUser => Hive.box(cachedUserBox);
  static Box get messageOutbox => Hive.box(messageOutboxBox);

  static Future<void> clearSessionData() async {
    final clears = <Future<int>>[];
    if (Hive.isBoxOpen(cachedUserBox)) clears.add(cachedUser.clear());
    if (Hive.isBoxOpen(messageOutboxBox)) clears.add(messageOutbox.clear());
    await Future.wait(clears);
  }
}
