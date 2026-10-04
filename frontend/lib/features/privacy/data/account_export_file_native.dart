import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' show Rect;

import 'package:file_selector/file_selector.dart' show getSaveLocation, XFile;
import 'package:path_provider/path_provider.dart';
import 'package:share_plus/share_plus.dart' hide XFile;

Future<Directory> _exportDirectory() async {
  final temporary = await getTemporaryDirectory();
  return Directory(
    '${temporary.path}${Platform.pathSeparator}bharatchat-account-exports',
  );
}

Future<void> clearOldAccountExports() async {
  final temporary = await getTemporaryDirectory();
  final directories = [
    await _exportDirectory(),
    if (Platform.isAndroid)
      Directory('${temporary.path}${Platform.pathSeparator}share_plus'),
  ];
  final cutoff = DateTime.now().subtract(const Duration(hours: 1));
  for (final directory in directories) {
    if (!await directory.exists()) continue;
    await for (final entry in directory.list(followLinks: false)) {
      final name = entry.path.split(Platform.pathSeparator).last;
      if (entry is File &&
          RegExp(r'^bharatchat-account-export-\d+\.zip$').hasMatch(name) &&
          (await entry.stat()).modified.isBefore(cutoff)) {
        await entry.delete();
      }
    }
  }
}

Future<bool> saveAccountExport(
  Uint8List bytes,
  String filename,
  Rect origin,
) async {
  if (!RegExp(r'^bharatchat-account-export-\d+\.zip$').hasMatch(filename)) {
    throw ArgumentError('Invalid account export filename');
  }
  if (Platform.isWindows || Platform.isLinux || Platform.isMacOS) {
    final location = await getSaveLocation(suggestedName: filename);
    if (location == null) return false;
    await XFile.fromData(
      bytes,
      mimeType: 'application/zip',
    ).saveTo(location.path);
    return true;
  }
  await clearOldAccountExports();
  final directory = await _exportDirectory();
  await directory.create(recursive: true);
  final file = File('${directory.path}${Platform.pathSeparator}$filename');
  try {
    await file.writeAsBytes(bytes, flush: true);
    final result = await SharePlus.instance.share(
      ShareParams(
        title: 'BharatChat account export',
        files: [XFile(file.path, mimeType: 'application/zip')],
        sharePositionOrigin: origin,
      ),
    );
    if (result.status == ShareResultStatus.dismissed) {
      await file.delete();
      return false;
    }
    // Receiving apps may read after the sheet closes. Retain this owned copy
    // briefly; startup cleanup removes copies older than one hour.
    return true;
  } catch (_) {
    if (await file.exists()) await file.delete();
    rethrow;
  }
}
