import 'dart:typed_data';
import 'dart:ui' show Rect;

import 'package:share_plus/share_plus.dart';

Future<void> clearOldAccountExports() async {}

Future<bool> saveAccountExport(
  Uint8List bytes,
  String filename,
  Rect origin,
) async {
  await XFile.fromData(
    bytes,
    mimeType: 'application/zip',
    name: filename,
  ).saveTo(filename);
  return true;
}
