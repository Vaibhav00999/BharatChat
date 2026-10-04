import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:bharatchat_mls/bharatchat_mls.dart';
import 'package:openmls/openmls.dart' show Openmls;
import 'package:test/test.dart';

const chatId = '10000000-0000-4000-8000-000000000001';
const otherChatId = '10000000-0000-4000-8000-000000000002';
String id(int number) =>
    '00000000-0000-4000-8000-${number.toString().padLeft(12, '0')}';
Uint8List bytes(String text) => Uint8List.fromList(utf8.encode(text));

class MemorySecrets implements MlsSecretStore {
  final values = <String, String>{};
  bool failReadyWrite = false;
  @override
  Future<String?> read(String key) async => values[key];
  @override
  Future<void> write(String key, String value) async {
    if (failReadyWrite && (jsonDecode(value) as Map)['status'] == 'ready') {
      failReadyWrite = false;
      throw const FileSystemException('Simulated secure-store write failure');
    }
    values[key] = value;
  }
}

void main() {
  late Directory root;
  late MemorySecrets secrets;
  late List<MlsDeviceSession> sessions;

  Future<MlsDeviceSession> open(int user, {int? device}) async {
    final session = await MlsDeviceSession.open(
      userId: id(user),
      deviceId: id(device ?? user + 100),
      stateDirectory: root,
      secrets: secrets,
    );
    sessions.add(session);
    return session;
  }

  Future<(MlsDeviceSession, MlsDeviceSession)> pair() async {
    final alice = await open(1);
    final bob = await open(2);
    await alice.createGroup(chatId);
    final addition = await alice.addMembers(chatId, [
      await bob.createKeyPackage(),
    ]);
    await bob.joinWelcome(chatId, addition.welcome);
    return (alice, bob);
  }

  setUp(() async {
    root = await Directory.systemTemp.createTemp('bharatchat-mls-test-');
    secrets = MemorySecrets();
    sessions = [];
  });
  tearDown(() async {
    for (final session in sessions.reversed) {
      await session.close();
    }
    final temp = await Directory.systemTemp.resolveSymbolicLinks();
    final resolved = await root.resolveSymbolicLinks();
    if (!resolved.startsWith(
      '$temp${Platform.pathSeparator}bharatchat-mls-test-',
    )) {
      throw StateError('Refusing to delete outside the owned test directory');
    }
    await root.delete(recursive: true);
  });
  tearDownAll(Openmls.cleanup);

  test('real MLS round trip in both directions and replay rejection', () async {
    final (alice, bob) = await pair();
    final ciphertext = await alice.encrypt(
      chatId,
      bytes('private-message-fixture'),
    );
    expect(
      utf8.decode(ciphertext, allowMalformed: true),
      isNot(contains('private-message-fixture')),
    );
    final received = await bob.receive(chatId, ciphertext);
    expect(received.messageType, ProcessedMessageType.application);
    expect(
      utf8.decode(received.applicationMessage!),
      'private-message-fixture',
    );
    await expectLater(bob.receive(chatId, ciphertext), throwsA(anything));
    final reply = await bob.encrypt(chatId, bytes('reply'));
    expect(
      (await alice.receive(chatId, reply)).applicationMessage,
      bytes('reply'),
    );
  });

  test(
    'tampered ciphertext fails without consuming the valid message',
    () async {
      final (alice, bob) = await pair();
      final original = await alice.encrypt(chatId, bytes('authentic'));
      final altered = Uint8List.fromList(original);
      altered[altered.length - 1] ^= 1;
      await expectLater(bob.receive(chatId, altered), throwsA(anything));
      expect(
        (await bob.receive(chatId, original)).applicationMessage,
        bytes('authentic'),
      );
    },
  );

  test('removed offline member cannot decrypt the next epoch', () async {
    final alice = await open(1);
    final bob = await open(2);
    final carol = await open(3);
    await alice.createGroup(chatId);
    final addition = await alice.addMembers(chatId, [
      await bob.createKeyPackage(),
      await carol.createKeyPackage(),
    ]);
    await bob.joinWelcome(chatId, addition.welcome);
    await carol.joinWelcome(chatId, addition.welcome);
    expect(await alice.memberIndices(chatId), [0, 1, 2]);
    final removal = await alice.removeMembers(chatId, [1]);
    await carol.receive(chatId, removal.commit);
    final next = await alice.encrypt(chatId, bytes('remaining members only'));
    await expectLater(bob.receive(chatId, next), throwsA(anything));
    expect(
      (await carol.receive(chatId, next)).applicationMessage,
      bytes('remaining members only'),
    );
    await bob.receive(chatId, removal.commit);
    await expectLater(bob.encrypt(chatId, bytes('removed')), throwsA(anything));
  });

  test('self update rotates epochs and discards past epoch secrets', () async {
    final (alice, bob) = await pair();
    final delayed = await alice.encrypt(chatId, bytes('old epoch'));
    final before = await alice.epoch(chatId);
    final rotation = await alice.rotateEpoch(chatId);
    await bob.receive(chatId, rotation.commit);
    expect(await alice.epoch(chatId), before + BigInt.one);
    expect(await bob.epoch(chatId), await alice.epoch(chatId));
    await expectLater(bob.receive(chatId, delayed), throwsA(anything));
    final current = await alice.encrypt(chatId, bytes('new epoch'));
    expect(
      (await bob.receive(chatId, current)).applicationMessage,
      bytes('new epoch'),
    );
  });

  test(
    'encrypted state and replay protection survive close and reopen',
    () async {
      final (alice, bob) = await pair();
      final originalPublicKey = bob.signingPublicKey;
      final first = await alice.encrypt(
        chatId,
        bytes('persistent-secret-fixture'),
      );
      await bob.receive(chatId, first);
      final path = bob.databasePath;
      await bob.close();
      final stored = await File(path).readAsBytes();
      expect(latin1.decode(stored), isNot(startsWith('SQLite format 3')));
      expect(
        latin1.decode(stored),
        isNot(contains('persistent-secret-fixture')),
      );
      final restored = await open(2);
      expect(restored.signingPublicKey, originalPublicKey);
      await expectLater(restored.receive(chatId, first), throwsA(anything));
      final second = await alice.encrypt(chatId, bytes('after restart'));
      expect(
        (await restored.receive(chatId, second)).applicationMessage,
        bytes('after restart'),
      );
    },
  );

  test(
    'wrong database key fails without replacing state or identity',
    () async {
      final (alice, bob) = await pair();
      final key = bob.secretStorageKey;
      final original = secrets.values[key]!;
      await bob.close();
      final modified = jsonDecode(original) as Map<String, dynamic>;
      final wrongKey = base64Url.decode(modified['databaseKey'] as String);
      wrongKey[0] ^= 1;
      modified['databaseKey'] = base64UrlEncode(wrongKey);
      secrets.values[key] = jsonEncode(modified);
      await expectLater(open(2), throwsA(anything));
      expect(secrets.values[key], jsonEncode(modified));
      secrets.values[key] = original;
      final restored = await open(2);
      final current = await alice.encrypt(chatId, bytes('key unchanged'));
      expect(
        (await restored.receive(chatId, current)).applicationMessage,
        bytes('key unchanged'),
      );
    },
  );

  test('missing secrets require recovery, never silent replacement', () async {
    final session = await open(1);
    final key = session.secretStorageKey;
    await session.close();
    secrets.values.remove(key);
    await expectLater(open(1), throwsStateError);
    expect(secrets.values.containsKey(key), isFalse);
    expect(await File(session.databasePath).exists(), isTrue);
  });

  test('missing initialized database requires recovery', () async {
    final session = await open(1);
    final material = secrets.values[session.secretStorageKey];
    await session.close();
    await File(session.databasePath).delete();
    await expectLater(open(1), throwsStateError);
    expect(await File(session.databasePath).exists(), isFalse);
    expect(secrets.values[session.secretStorageKey], material);
  });

  test('secrets cannot be substituted between accounts', () async {
    final alice = await open(1);
    final bob = await open(2);
    final key = bob.secretStorageKey;
    await bob.close();
    secrets.values[key] = secrets.values[alice.secretStorageKey]!;
    await expectLater(open(2), throwsFormatException);
  });

  test('only one state writer opens; other devices remain isolated', () async {
    final first = await open(1);
    await expectLater(open(1), throwsStateError);
    final secondDevice = await open(1, device: 999);
    expect(secondDevice.databasePath, isNot(first.databasePath));
    expect(secondDevice.signingPublicKey, isNot(first.signingPublicKey));
  });

  test(
    'failed secure-store completion retries with the same identity',
    () async {
      secrets.failReadyWrite = true;
      await expectLater(open(1), throwsA(isA<FileSystemException>()));
      final material = jsonDecode(secrets.values.values.single) as Map;
      expect(material['status'], 'initializing');
      final restored = await open(1);
      expect(
        restored.signingPublicKey,
        base64Url.decode(material['publicKey'] as String),
      );
      expect(
        (jsonDecode(secrets.values.values.single) as Map)['status'],
        'ready',
      );
    },
  );

  test(
    'Welcome is bound to the chat, failed inspection does not consume it',
    () async {
      final alice = await open(1);
      final bob = await open(2);
      await alice.createGroup(chatId);
      final addition = await alice.addMembers(chatId, [
        await bob.createKeyPackage(),
      ]);
      await expectLater(
        bob.joinWelcome(otherChatId, addition.welcome),
        throwsFormatException,
      );
      await bob.joinWelcome(chatId, addition.welcome);
      await expectLater(
        bob.joinWelcome(chatId, addition.welcome),
        throwsA(anything),
      );
      final valid = await alice.encrypt(chatId, bytes('correct group'));
      expect(
        (await bob.receive(chatId, valid)).applicationMessage,
        bytes('correct group'),
      );
    },
  );

  test(
    'invalid identity is rejected before any file or secret is created',
    () async {
      await expectLater(
        MlsDeviceSession.open(
          userId: '../outside',
          deviceId: id(1),
          stateDirectory: root,
          secrets: secrets,
        ),
        throwsArgumentError,
      );
      expect(secrets.values, isEmpty);
      expect(await root.list().toList(), isEmpty);
    },
  );

  test(
    'closed sessions reject further operations and close is idempotent',
    () async {
      final session = await open(1);
      await session.close();
      await session.close();
      await expectLater(session.createKeyPackage(), throwsStateError);
      await expectLater(
        session.encrypt(chatId, bytes('closed')),
        throwsStateError,
      );
    },
  );
}
