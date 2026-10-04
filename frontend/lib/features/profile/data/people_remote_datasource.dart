import 'package:dio/dio.dart';

class Person {
  final String id;
  final String displayName;
  final String? username;
  const Person({required this.id, required this.displayName, this.username});
  factory Person.fromJson(Map<String, dynamic> json) => Person(
    id: json['id'] as String,
    displayName: json['displayName'] as String,
    username: json['username'] as String?,
  );
}

class BlockedPage {
  final List<Person> users;
  final String? nextCursor;
  const BlockedPage(this.users, this.nextCursor);
}

class PeopleRemoteDataSource {
  final Dio _dio;
  PeopleRemoteDataSource(this._dio);

  Future<Person> resolve(String username) async {
    final response = await _dio.post(
      '/users/resolve',
      data: {'username': username},
    );
    return Person.fromJson(response.data as Map<String, dynamic>);
  }

  Future<BlockedPage> blocked({String? after}) async {
    final response = await _dio.get(
      '/users/me/blocked',
      queryParameters: {if (after != null) 'after': after},
    );
    final data = response.data as Map<String, dynamic>;
    final next = data['nextCursor'] as String?;
    return BlockedPage(
      (data['users'] as List)
          .map((p) => Person.fromJson(p as Map<String, dynamic>))
          .toList(),
      next == '' ? null : next,
    );
  }

  Future<void> block(String id) async {
    await _dio.put('/users/me/blocked/$id');
  }

  Future<void> unblock(String id) async {
    await _dio.delete('/users/me/blocked/$id');
  }

  Future<void> report(String id, String reason, String details) async {
    await _dio.post(
      '/users/me/reports',
      data: {'userId': id, 'reason': reason, 'details': details},
    );
  }
}

String peopleError(Object error) {
  if (error is DioException) {
    switch (error.response?.statusCode) {
      case 404:
        return 'User not found';
      case 429:
        return 'Too many requests. Please try again later.';
      case 403:
        return 'This conversation is unavailable';
      case 400:
        return 'Check the details and try again';
    }
  }
  return 'Could not complete the request. Please try again.';
}
