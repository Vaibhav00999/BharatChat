import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:bharatchat/features/profile/data/people_remote_datasource.dart';
import 'package:bharatchat/features/profile/presentation/screens/people_screens.dart';

class FakePeople implements PeopleRemoteDataSource {
  String? resolved;
  String? unblocked;
  String? reportReason;
  final person = const Person(
    id: 'person-1',
    displayName: 'Asha',
    username: 'asha',
  );
  @override
  Future<Person> resolve(String username) async {
    resolved = username;
    return person;
  }

  @override
  Future<BlockedPage> blocked({String? after}) async =>
      BlockedPage([person], null);
  @override
  Future<void> block(String id) async {}
  @override
  Future<void> unblock(String id) async {
    unblocked = id;
  }

  @override
  Future<void> report(String id, String reason, String details) async {
    reportReason = reason;
  }
}

void main() {
  testWidgets('exact username lookup validates input and displays a person', (
    tester,
  ) async {
    final people = FakePeople();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [peopleRemoteProvider.overrideWithValue(people)],
        child: const MaterialApp(home: NewChatScreen()),
      ),
    );
    await tester.tap(find.byTooltip('Find user'));
    await tester.pumpAndSettle();
    expect(people.resolved, isNull);
    expect(
      find.text('Enter a username with 3-30 letters or digits'),
      findsOneWidget,
    );
    await tester.enterText(find.byType(TextField), '@ASHA');
    await tester.tap(find.byTooltip('Find user'));
    await tester.pumpAndSettle();
    expect(people.resolved, 'asha');
    expect(find.text('Asha'), findsOneWidget);
    expect(find.byTooltip('Start chat'), findsOneWidget);
  });

  testWidgets('unblocking requires confirmation and removes the user', (
    tester,
  ) async {
    final people = FakePeople();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [peopleRemoteProvider.overrideWithValue(people)],
        child: const MaterialApp(home: BlockedUsersScreen()),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Unblock'));
    await tester.pumpAndSettle();
    expect(people.unblocked, isNull);
    await tester.tap(find.text('Unblock').last);
    await tester.pumpAndSettle();
    expect(people.unblocked, 'person-1');
    expect(find.text('No blocked users'), findsOneWidget);
  });

  testWidgets('report dialog discloses submitted data before sending', (
    tester,
  ) async {
    final people = FakePeople();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [peopleRemoteProvider.overrideWithValue(people)],
        child: MaterialApp(
          home: Builder(
            builder: (context) => Scaffold(
              body: TextButton(
                onPressed: () => reportPerson(context, 'person-1'),
                child: const Text('Report'),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('Report'));
    await tester.pumpAndSettle();
    expect(
      find.textContaining('Chat messages are not attached.'),
      findsOneWidget,
    );
    expect(people.reportReason, isNull);
    await tester.tap(find.text('Submit report'));
    await tester.pumpAndSettle();
    expect(people.reportReason, 'spam');
    expect(find.text('Report submitted'), findsOneWidget);
  });
}
