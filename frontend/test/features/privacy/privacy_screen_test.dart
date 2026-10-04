import 'dart:async';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:bharatchat/features/auth/presentation/providers/auth_providers.dart';
import 'package:bharatchat/features/privacy/data/privacy_remote_datasource.dart';
import 'package:bharatchat/features/privacy/domain/privacy_dashboard.dart';
import 'package:bharatchat/features/privacy/presentation/privacy_providers.dart';
import 'package:bharatchat/features/privacy/presentation/privacy_screen.dart';

import '../../core/network/dio_client_test.dart' show MemoryTokens;
import '../auth/presentation/providers/auth_controller_test.dart'
    show FakeAuthRepository;

class FakePrivacySource extends PrivacyRemoteDataSource {
  int exports = 0;
  final pending = Completer<Uint8List>();
  FakePrivacySource() : super(Dio(), MemoryTokens());
  @override
  Future<PrivacyDashboard> load() async => PrivacyDashboard(
    settings: PrivacySettings.fromJson({
      'privacyLastSeen': 'nobody',
      'privacyAvatar': 'contacts',
      'privacyAbout': 'contacts',
      'privacyPhone': 'nobody',
      'allowGroupAdds': 'contacts',
      'privacyReadReceipts': false,
      'discoverableByPhone': false,
      'securityNotifications': true,
    }),
    devices: const [],
  );
  @override
  Future<Uint8List> exportAccount({CancelToken? cancelToken}) {
    exports++;
    return pending.future;
  }
}

void main() {
  Future<void> showPrivacy(
    WidgetTester tester,
    FakePrivacySource source,
    AccountExportSaver saver,
  ) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          privacyRemoteDataSourceProvider.overrideWithValue(source),
          authRepositoryProvider.overrideWithValue(FakeAuthRepository()),
          accountExportSaverProvider.overrideWithValue(saver),
        ],
        child: const MaterialApp(home: PrivacyScreen()),
      ),
    );
    await tester.pumpAndSettle();
    await tester.scrollUntilVisible(find.text('Export account data'), 250);
  }

  testWidgets('export asks for consent before downloading personal data', (
    tester,
  ) async {
    final source = FakePrivacySource();
    var saves = 0;
    await showPrivacy(tester, source, (bytes, filename, origin) async {
      saves++;
      expect(filename, matches(r'^bharatchat-account-export-\d+\.zip$'));
      expect(bytes, [0x50, 0x4b, 3, 4]);
      return true;
    });
    await tester.tap(find.text('Export account data'));
    await tester.pumpAndSettle();
    expect(find.textContaining('not password-protected'), findsOneWidget);
    expect(source.exports, 0);
    await tester.tap(find.text('Export'));
    await tester.pump();
    source.pending.complete(Uint8List.fromList([0x50, 0x4b, 3, 4]));
    await tester.pumpAndSettle();
    expect(source.exports, 1);
    expect(saves, 1);
    expect(find.text('Export account data'), findsOneWidget);
  });

  testWidgets('cancelled export never invokes the file saver', (tester) async {
    final source = FakePrivacySource();
    var saves = 0;
    await showPrivacy(tester, source, (_, __, ___) async {
      saves++;
      return true;
    });
    await tester.tap(find.text('Export account data'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Export'));
    await tester.pump();
    await tester.tap(find.byTooltip('Cancel export'));
    source.pending.complete(Uint8List.fromList([0x50, 0x4b, 3, 4]));
    await tester.pumpAndSettle();
    expect(saves, 0);
  });

  testWidgets('deletion stays disabled until exact DELETE confirmation', (
    tester,
  ) async {
    final source = FakePrivacySource();
    await showPrivacy(tester, source, (_, __, ___) async => true);
    await tester.scrollUntilVisible(find.text('Delete account'), 200);
    await tester.tap(find.text('Delete account'));
    await tester.pumpAndSettle();
    FilledButton deleteButton() => tester.widget<FilledButton>(
      find.widgetWithText(FilledButton, 'Delete'),
    );
    expect(deleteButton().onPressed, isNull);
    await tester.enterText(find.byType(TextField), 'delete');
    await tester.pump();
    expect(deleteButton().onPressed, isNull);
    await tester.enterText(find.byType(TextField), 'DELETE');
    await tester.pump();
    expect(deleteButton().onPressed, isNotNull);
    await tester.tap(find.text('Cancel'));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
  });
}
