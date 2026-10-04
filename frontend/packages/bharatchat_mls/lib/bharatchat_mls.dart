import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import 'package:openmls/openmls.dart';

export 'package:openmls/openmls.dart'
    show
        AddMembersResult,
        CommitResult,
        ProcessedMessageResult,
        ProcessedMessageType;

/// Implement with platform secure storage, never preferences or the MLS database.
abstract interface class MlsSecretStore {
  Future<String?> read(String key);
  Future<void> write(String key, String value);
}

/// Native, prelaunch MLS state. Transport, identity trust, and commit delivery
/// must be reviewed before this component can be enabled in the shipping app.
class MlsDeviceSession {
  static const _suite = MlsCiphersuite.mls128DhkemX25519Aes128GcmSha256Ed25519;
  static final _uuid = RegExp(
    r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
  );
  static final _openPaths = <String>{};
  static Future<void>? _initialization;
  static const _config = MlsGroupConfig(
    ciphersuite: _suite,
    wireFormatPolicy: MlsWireFormatPolicy.ciphertext,
    useRatchetTreeExtension: true,
    maxPastEpochs: 0,
    paddingSize: 256,
    senderRatchetMaxOutOfOrder: 128,
    senderRatchetMaxForwardDistance: 1000,
    numberOfResumptionPsks: 0,
  );

  final MlsEngine _engine;
  final Uint8List _signer;
  final Uint8List _publicKey;
  final Uint8List _identity;
  final RandomAccessFile _lock;
  final String databasePath;
  final String secretStorageKey;
  Future<void>? _closing;

  MlsDeviceSession._(
    this._engine,
    this._signer,
    this._publicKey,
    this._identity,
    this._lock,
    this.databasePath,
    this.secretStorageKey,
  );

  Uint8List get signingPublicKey => Uint8List.fromList(_publicKey);

  static String _canonicalId(String value) {
    final normalized = value.toLowerCase();
    if (!_uuid.hasMatch(normalized)) throw ArgumentError('Expected a UUID');
    return normalized;
  }

  static MlsCapabilities get _capabilities => MlsCapabilities(
    versions: Uint16List.fromList([1]),
    ciphersuites: Uint16List.fromList([0x0001]),
    extensions: Uint16List(0),
    proposals: Uint16List(0),
    credentials: Uint16List.fromList([1]),
  );

  static Future<MlsDeviceSession> open({
    required String userId,
    required String deviceId,
    required Directory stateDirectory,
    required MlsSecretStore secrets,
  }) async {
    userId = _canonicalId(userId);
    deviceId = _canonicalId(deviceId);
    await stateDirectory.create(recursive: true);
    final root = await stateDirectory.resolveSymbolicLinks();
    final path = '$root${Platform.pathSeparator}$userId-$deviceId.sqlite';
    final storageKey = 'bharatchat_mls_v1:$userId:$deviceId';
    if (!_openPaths.add(path)) throw StateError('MLS state is already open');

    RandomAccessFile? lock;
    MlsEngine? engine;
    Uint8List? signer;
    Uint8List? databaseKey;
    try {
      lock = await File('$path.lock').open(mode: FileMode.append);
      await lock.lock(FileLock.exclusive);
      await (_initialization ??= Openmls.init());
      var encoded = await secrets.read(storageKey);
      if (encoded == null) {
        for (final suffix in ['', '-wal', '-shm']) {
          if (await File('$path$suffix').exists()) {
            throw StateError(
              'MLS secrets are missing; explicit recovery is required',
            );
          }
        }
        final pair = MlsSignatureKeyPair.generate(ciphersuite: _suite);
        final privateKey = pair.privateKey();
        try {
          signer = serializeSigner(
            ciphersuite: _suite,
            privateKey: privateKey,
            publicKey: pair.publicKey(),
          );
          final random = Random.secure();
          databaseKey = Uint8List.fromList(
            List.generate(32, (_) => random.nextInt(256)),
          );
          encoded = jsonEncode({
            'version': 1,
            'userId': userId,
            'deviceId': deviceId,
            'status': 'initializing',
            'databaseKey': base64UrlEncode(databaseKey),
            'signer': base64UrlEncode(signer),
            'publicKey': base64UrlEncode(pair.publicKey()),
          });
          await secrets.write(storageKey, encoded);
        } finally {
          privateKey.fillRange(0, privateKey.length, 0);
          pair.dispose();
        }
      }
      final material = jsonDecode(encoded) as Map<String, dynamic>;
      if (material['version'] != 1 ||
          material['userId'] != userId ||
          material['deviceId'] != deviceId) {
        throw const FormatException('MLS secret identity does not match');
      }
      if (material['status'] != 'initializing' &&
          material['status'] != 'ready') {
        throw const FormatException('Invalid MLS initialization state');
      }
      if (material['status'] == 'ready' && !await File(path).exists()) {
        throw StateError(
          'MLS database is missing; explicit recovery is required',
        );
      }
      databaseKey?.fillRange(0, databaseKey.length, 0);
      databaseKey = base64Url.decode(material['databaseKey'] as String);
      signer?.fillRange(0, signer.length, 0);
      signer = base64Url.decode(material['signer'] as String);
      final publicKey = base64Url.decode(material['publicKey'] as String);
      if (databaseKey.length != 32 ||
          publicKey.length != 32 ||
          signer.length < 32 ||
          signer.length > 4096) {
        throw const FormatException('Invalid MLS secret material');
      }
      engine = await MlsEngine.create(dbPath: path, encryptionKey: databaseKey);
      // Persist completion only after SQLCipher has successfully opened, so a
      // failed first open can retry without replacing its saved identity.
      if (material['status'] == 'initializing') {
        material['status'] = 'ready';
        await secrets.write(storageKey, jsonEncode(material));
      }
      return MlsDeviceSession._(
        engine,
        signer,
        publicKey,
        Uint8List.fromList(
          utf8.encode(
            jsonEncode({'version': 1, 'userId': userId, 'deviceId': deviceId}),
          ),
        ),
        lock,
        path,
        storageKey,
      );
    } catch (_) {
      signer?.fillRange(0, signer.length, 0);
      try {
        if (engine != null) await engine.close();
      } finally {
        try {
          if (lock != null) await lock.close();
        } finally {
          _openPaths.remove(path);
        }
      }
      rethrow;
    } finally {
      databaseKey?.fillRange(0, databaseKey.length, 0);
    }
  }

  void _requireOpen() {
    if (_closing != null) throw StateError('MLS session is closed');
  }

  Uint8List _groupId(String chatId) {
    _requireOpen();
    return Uint8List.fromList(utf8.encode(_canonicalId(chatId)));
  }

  Future<Uint8List> createKeyPackage() async {
    _requireOpen();
    final result = await _engine.createKeyPackageWithOptions(
      ciphersuite: _suite,
      signerBytes: _signer,
      credentialIdentity: _identity,
      signerPublicKey: _publicKey,
      options: KeyPackageOptions(
        lastResort: false,
        lifetimeSeconds: BigInt.from(7 * 24 * 60 * 60),
        capabilities: _capabilities,
      ),
    );
    return result.keyPackageBytes;
  }

  Future<void> createGroup(String chatId) async {
    await _engine.createGroupWithBuilder(
      config: _config,
      signerBytes: _signer,
      credentialIdentity: _identity,
      signerPublicKey: _publicKey,
      groupId: _groupId(chatId),
      capabilities: _capabilities,
    );
  }

  Future<AddMembersResult> addMembers(
    String chatId,
    List<Uint8List> authenticatedKeyPackages,
  ) {
    if (authenticatedKeyPackages.isEmpty ||
        authenticatedKeyPackages.length > 255) {
      throw ArgumentError('Expected 1-255 authenticated device key packages');
    }
    var total = 0;
    for (final package in authenticatedKeyPackages) {
      if (package.isEmpty || package.length > 16 * 1024) {
        throw ArgumentError('Invalid key package length');
      }
      total += package.length;
    }
    if (total > 1024 * 1024) {
      throw ArgumentError('Key package batch is too large');
    }
    return _engine.addMembers(
      groupIdBytes: _groupId(chatId),
      signerBytes: _signer,
      keyPackagesBytes: authenticatedKeyPackages,
    );
  }

  Future<void> joinWelcome(String chatId, Uint8List welcome) async {
    final expected = _groupId(chatId);
    if (welcome.isEmpty || welcome.length > 1024 * 1024) {
      throw ArgumentError('Invalid Welcome length');
    }
    final info = await _engine.inspectWelcome(
      config: _config,
      welcomeBytes: welcome,
    );
    if (info.ciphersuite != _suite ||
        !_sameBytes(info.groupId, expected) ||
        info.pskCount != 0) {
      throw const FormatException(
        'Welcome does not match the permitted MLS group',
      );
    }
    await _engine.joinGroupFromWelcome(
      config: _config,
      welcomeBytes: welcome,
      signerBytes: _signer,
    );
  }

  Future<Uint8List> encrypt(String chatId, Uint8List plaintext) async {
    if (plaintext.isEmpty || plaintext.length > 8000) {
      throw ArgumentError('Invalid message length');
    }
    final result = await _engine.createMessage(
      groupIdBytes: _groupId(chatId),
      signerBytes: _signer,
      message: plaintext,
    );
    return result.ciphertext;
  }

  Future<ProcessedMessageResult> receive(String chatId, Uint8List message) {
    if (message.isEmpty || message.length > 1024 * 1024) {
      throw ArgumentError('Invalid MLS frame length');
    }
    return _engine.processMessage(
      groupIdBytes: _groupId(chatId),
      messageBytes: message,
    );
  }

  Future<CommitResult> removeMembers(
    String chatId,
    List<int> authorizedMemberIndices,
  ) => _engine.removeMembers(
    groupIdBytes: _groupId(chatId),
    signerBytes: _signer,
    memberIndices: authorizedMemberIndices,
  );

  Future<CommitResult> rotateEpoch(String chatId) =>
      _engine.selfUpdate(groupIdBytes: _groupId(chatId), signerBytes: _signer);

  Future<BigInt> epoch(String chatId) =>
      _engine.groupEpoch(groupIdBytes: _groupId(chatId));

  Future<List<int>> memberIndices(String chatId) async =>
      (await _engine.groupMembers(
        groupIdBytes: _groupId(chatId),
      )).map((member) => member.index).toList();

  Future<void> close() => _closing ??= _close();

  Future<void> _close() async {
    try {
      await _engine.close();
    } finally {
      _signer.fillRange(0, _signer.length, 0);
      try {
        await _lock.close();
      } finally {
        _openPaths.remove(databasePath);
      }
    }
  }

  static bool _sameBytes(List<int> a, List<int> b) {
    if (a.length != b.length) return false;
    for (var index = 0; index < a.length; index++) {
      if (a[index] != b[index]) return false;
    }
    return true;
  }
}
