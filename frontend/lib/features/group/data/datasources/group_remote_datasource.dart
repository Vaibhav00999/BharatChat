import 'package:dio/dio.dart';

import '../../domain/entities/group_info.dart';

class GroupRemoteDataSource {
  final Dio _dio;
  GroupRemoteDataSource(this._dio);
  Map<String, dynamic> _map(dynamic data) =>
      Map<String, dynamic>.from(data as Map);
  Future<GroupInfo> create({
    required String name,
    String description = '',
    required List<String> memberIds,
  }) async => GroupInfo.fromJson(
    _map(
      (await _dio.post(
        '/groups',
        data: {
          'name': name,
          'description': description,
          'memberUserIds': memberIds,
        },
      )).data,
    ),
  );
  Future<GroupInfo> getInfo(String id) async =>
      GroupInfo.fromJson(_map((await _dio.get('/groups/$id')).data));
  Future<List<GroupMember>> members(String id) async {
    final r = await _dio.get('/groups/$id/members');
    return ((r.data as Map)['members'] as List)
        .map((e) => GroupMember.fromJson(_map(e)))
        .toList();
  }

  Future<GroupInfo> join(String code) async => GroupInfo.fromJson(
    _map((await _dio.post('/groups/join', data: {'inviteCode': code})).data),
  );
  Future<void> updateInfo(
    String id, {
    String? name,
    String? description,
    String? iconUrl,
  }) => _dio.patch(
    '/groups/$id',
    data: {
      if (name != null) 'name': name,
      if (description != null) 'description': description,
      if (iconUrl != null) 'iconUrl': iconUrl,
    },
  );
  Future<void> addMember(String id, String user) =>
      _dio.post('/groups/$id/members', data: {'userId': user});
  Future<void> removeMember(String id, String user) =>
      _dio.delete('/groups/$id/members/$user');
  Future<void> setAdmin(String id, String user, bool value) =>
      _dio.patch('/groups/$id/members/$user/admin', data: {'isAdmin': value});
  Future<void> transfer(String id, String user) => _dio.post(
    '/groups/$id/transfer-ownership',
    data: {'newOwnerUserId': user},
  );
  Future<void> leave(String id) => _dio.post('/groups/$id/leave');
  Future<void> setAdminsPost(String id, bool value) =>
      _dio.patch('/groups/$id/only-admins-can-post', data: {'value': value});
  Future<void> setAdminsEdit(String id, bool value) => _dio.patch(
    '/groups/$id/only-admins-can-edit-info',
    data: {'value': value},
  );
  Future<String> regenerateCode(String id) async =>
      (_map(
            (await _dio.post('/groups/$id/invite-code/regenerate')).data,
          )['inviteCode']
          as String);
  Future<void> setInviteEnabled(String id, bool value) =>
      _dio.patch('/groups/$id/invite-code/enabled', data: {'value': value});
}
