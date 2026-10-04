import 'dart:convert';
import 'dart:math';

import 'package:cryptography/cryptography.dart';
import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

import '../../../core/storage/secure_storage.dart';

const _deviceKeyMaterialStorageKeyPrefix = 'bharatchat_e2ee_device_keys_v1';

class VerifiedDeviceKeyBundle {
  final String deviceId;
  final String userId;
  final int registrationId;
  final List<int> identityAgreementPublicKey;
  final List<int> identitySigningPublicKey;
  final int signedPreKeyId;
  final List<int> signedPreKeyPublicKey;
  final int? oneTimePreKeyId;
  final List<int>? oneTimePreKeyPublicKey;

  const VerifiedDeviceKeyBundle({
    required this.deviceId,
    required this.userId,
    required this.registrationId,
    required this.identityAgreementPublicKey,
    required this.identitySigningPublicKey,
    required this.signedPreKeyId,
    required this.signedPreKeyPublicKey,
    this.oneTimePreKeyId,
    this.oneTimePreKeyPublicKey,
  });
}

/// Owns this installation's private identity material. Private bytes are written
/// only to platform secure storage; only signed public material is uploaded.
/// Browser builds fail closed until a non-exportable WebCrypto key store is wired.
class DeviceKeyManager {
  final Dio _dio;
  final SecureTokenStorage _secureStorage;
  final X25519 _agreement = X25519();
  final Ed25519 _signing = Ed25519();

  DeviceKeyManager(this._dio, this._secureStorage);

  Future<void> clearLocalIdentity(String userId) => _secureStorage.deleteSecret(
    '${_deviceKeyMaterialStorageKeyPrefix}_$userId',
  );

  Future<void> ensurePublished(String userId) async {
    if (kIsWeb) {
      throw UnsupportedError(
        'End-to-end encryption requires non-exportable browser key storage',
      );
    }

    if (userId.isEmpty) throw StateError('Authenticated user is unavailable');
    final storageKey = '${_deviceKeyMaterialStorageKeyPrefix}_$userId';
    var encoded = await _secureStorage.readSecret(storageKey);
    if (encoded == null) {
      encoded = jsonEncode(await _generateKeyMaterial());
      await _secureStorage.writeSecret(storageKey, encoded);
    }

    final material = Map<String, dynamic>.from(jsonDecode(encoded) as Map);
    await _dio.put('/privacy/keys', data: _publicBundle(material));
  }

  Future<List<VerifiedDeviceKeyBundle>> claimVerifiedBundles(
    String userId,
  ) async {
    final response = await _dio.get('/privacy/users/$userId/key-bundles');
    final rawBundles =
        (response.data as Map<String, dynamic>)['keyBundles'] as List;
    final verified = <VerifiedDeviceKeyBundle>[];
    for (final raw in rawBundles) {
      final json = Map<String, dynamic>.from(raw as Map);
      final signingKey = _decode(json['identitySigningPublicKey'] as String);
      final signedPreKey = _decode(json['signedPreKeyPublicKey'] as String);
      final signature = Signature(
        _decode(json['signedPreKeySignature'] as String),
        publicKey: SimplePublicKey(signingKey, type: KeyPairType.ed25519),
      );
      if (!await _signing.verify(signedPreKey, signature: signature)) {
        throw const FormatException('Remote device key signature is invalid');
      }

      final oneTime = json['oneTimePreKey'] as Map<String, dynamic>?;
      verified.add(
        VerifiedDeviceKeyBundle(
          deviceId: json['deviceId'] as String,
          userId: json['userId'] as String,
          registrationId: json['registrationId'] as int,
          identityAgreementPublicKey: _decode(
            json['identityAgreementPublicKey'] as String,
          ),
          identitySigningPublicKey: signingKey,
          signedPreKeyId: json['signedPreKeyId'] as int,
          signedPreKeyPublicKey: signedPreKey,
          oneTimePreKeyId: oneTime?['keyId'] as int?,
          oneTimePreKeyPublicKey: oneTime == null
              ? null
              : _decode(oneTime['publicKey'] as String),
        ),
      );
    }
    return verified;
  }

  Future<Map<String, dynamic>> _generateKeyMaterial() async {
    final identityAgreement = await _agreement.newKeyPair();
    final identitySigning = await _signing.newKeyPair();
    final signedPreKey = await _agreement.newKeyPair();
    final signedPreKeyPublic = await signedPreKey.extractPublicKey();
    final signature = await _signing.sign(
      signedPreKeyPublic.bytes,
      keyPair: identitySigning,
    );

    final oneTimePreKeys = <Map<String, dynamic>>[];
    for (var index = 0; index < 100; index++) {
      final pair = await _agreement.newKeyPair();
      final publicKey = await pair.extractPublicKey();
      oneTimePreKeys.add({
        'keyId': index,
        'privateKey': _encode(await pair.extractPrivateKeyBytes()),
        'publicKey': _encode(publicKey.bytes),
      });
      pair.destroy();
    }

    final identityAgreementPublic = await identityAgreement.extractPublicKey();
    final identitySigningPublic = await identitySigning.extractPublicKey();
    final result = <String, dynamic>{
      'registrationId': Random.secure().nextInt(16380) + 1,
      'identityAgreementPrivateKey': _encode(
        await identityAgreement.extractPrivateKeyBytes(),
      ),
      'identityAgreementPublicKey': _encode(identityAgreementPublic.bytes),
      'identitySigningPrivateKey': _encode(
        await identitySigning.extractPrivateKeyBytes(),
      ),
      'identitySigningPublicKey': _encode(identitySigningPublic.bytes),
      'signedPreKeyId': 1,
      'signedPreKeyPrivateKey': _encode(
        await signedPreKey.extractPrivateKeyBytes(),
      ),
      'signedPreKeyPublicKey': _encode(signedPreKeyPublic.bytes),
      'signedPreKeySignature': _encode(signature.bytes),
      'keyVersion': 1,
      'oneTimePreKeys': oneTimePreKeys,
    };
    identityAgreement.destroy();
    identitySigning.destroy();
    signedPreKey.destroy();
    return result;
  }

  Map<String, dynamic> _publicBundle(Map<String, dynamic> material) {
    final publicPreKeys = <Map<String, dynamic>>[];
    for (final rawKey in material['oneTimePreKeys'] as List) {
      final key = Map<String, dynamic>.from(rawKey as Map);
      publicPreKeys.add({'keyId': key['keyId'], 'publicKey': key['publicKey']});
    }

    return {
      'registrationId': material['registrationId'],
      'identityAgreementPublicKey': material['identityAgreementPublicKey'],
      'identitySigningPublicKey': material['identitySigningPublicKey'],
      'signedPreKeyId': material['signedPreKeyId'],
      'signedPreKeyPublicKey': material['signedPreKeyPublicKey'],
      'signedPreKeySignature': material['signedPreKeySignature'],
      'keyVersion': material['keyVersion'],
      'oneTimePreKeys': publicPreKeys,
    };
  }

  String _encode(List<int> bytes) =>
      base64Url.encode(bytes).replaceAll('=', '');

  List<int> _decode(String value) {
    final padded = value.padRight((value.length + 3) ~/ 4 * 4, '=');
    return base64Url.decode(padded);
  }
}
