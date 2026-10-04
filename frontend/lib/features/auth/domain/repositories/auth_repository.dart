import '../../../../core/network/api_result.dart';
import '../entities/auth_session.dart';
import '../entities/device_info.dart';

abstract class AuthRepository {
  Future<ApiResult<void>> requestOtp({
    required String phoneNumber,
    required String countryCode,
  });
  Future<ApiResult<AuthSession>> verifyOtp({
    required String phoneNumber,
    required String countryCode,
    required String code,
    required DeviceInfoPayload device,
  });
  Future<ApiResult<AuthSession>> refreshSession();
  Future<ApiResult<void>> logout();
  Future<bool> hasPersistedSession();
}
