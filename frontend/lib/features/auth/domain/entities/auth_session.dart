class AuthSession {
  final String userId;
  final bool isNewUser;
  final String accessToken;
  final String refreshToken;
  final DateTime accessTokenExpiresAt;
  const AuthSession({
    required this.userId,
    required this.isNewUser,
    required this.accessToken,
    required this.refreshToken,
    required this.accessTokenExpiresAt,
  });
  factory AuthSession.fromJson(Map<String, dynamic> json) => AuthSession(
    userId: json['userId'] as String,
    isNewUser: json['isNewUser'] as bool? ?? false,
    accessToken: json['accessToken'] as String,
    refreshToken: json['refreshToken'] as String,
    accessTokenExpiresAt: DateTime.parse(
      json['accessTokenExpiresAt'] as String,
    ),
  );
}
