import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';
import 'core/config/env.dart';
import 'core/storage/hive_boxes.dart';
import 'features/privacy/data/account_export_file.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  Env.validate();
  await HiveBoxes.init();
  try {
    await clearOldAccountExports();
  } catch (_) {
    // A temporary-directory failure must not prevent signing in.
  }
  runApp(const ProviderScope(child: BharatChatApp()));
}
