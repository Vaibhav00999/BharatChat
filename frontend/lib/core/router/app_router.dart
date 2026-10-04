import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/presentation/providers/auth_providers.dart';
import '../../features/auth/presentation/screens/otp_verify_screen.dart';
import '../../features/auth/presentation/screens/phone_entry_screen.dart';
import '../../features/chat/presentation/screens/chat_screen.dart';
import '../../features/chat_list/presentation/providers/chat_list_providers.dart';
import '../../features/chat_list/presentation/screens/chat_list_screen.dart';
import '../../features/group/presentation/screens/create_group_screen.dart';
import '../../features/group/presentation/screens/group_info_screen.dart';
import '../../features/group/presentation/screens/join_group_screen.dart';
import '../../features/profile/presentation/screens/profile_setup_screen.dart';
import '../../features/privacy/presentation/privacy_screen.dart';
import '../../features/profile/presentation/screens/people_screens.dart';

class SplashScreen extends StatelessWidget {
  const SplashScreen({super.key});
  @override
  Widget build(BuildContext context) =>
      const Scaffold(body: Center(child: CircularProgressIndicator()));
}

class HomeShellScreen extends ConsumerWidget {
  const HomeShellScreen({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.watch(webSocketClientProvider);
    return const ChatListScreen();
  }
}

final appRouterProvider = Provider<GoRouter>((ref) {
  final refresh = ValueNotifier<int>(0);
  ref.listen(authControllerProvider, (_, __) => refresh.value++);
  final router = GoRouter(
    refreshListenable: refresh,
    initialLocation: '/splash',
    redirect: (context, state) {
      final auth = ref.read(authControllerProvider);
      final authRoute = state.matchedLocation.startsWith('/auth');
      switch (auth.status) {
        case AuthStatus.unknown:
          return '/splash';
        case AuthStatus.unauthenticated:
          return authRoute ? null : '/auth/phone-entry';
        case AuthStatus.otpSent:
          return state.matchedLocation == '/auth/otp-verify'
              ? null
              : '/auth/otp-verify';
        case AuthStatus.needsProfileSetup:
          return state.matchedLocation == '/profile/setup'
              ? null
              : '/profile/setup';
        case AuthStatus.authenticated:
          return authRoute ||
                  state.matchedLocation == '/splash' ||
                  state.matchedLocation == '/profile/setup'
              ? '/home'
              : null;
      }
    },
    routes: [
      GoRoute(path: '/splash', builder: (_, __) => const SplashScreen()),
      GoRoute(
        path: '/auth/phone-entry',
        builder: (_, __) => const PhoneEntryScreen(),
      ),
      GoRoute(
        path: '/auth/otp-verify',
        builder: (_, __) => const OtpVerifyScreen(),
      ),
      GoRoute(
        path: '/profile/setup',
        builder: (_, __) => const ProfileSetupScreen(),
      ),
      GoRoute(path: '/home', builder: (_, __) => const HomeShellScreen()),
      GoRoute(path: '/privacy', builder: (_, __) => const PrivacyScreen()),
      GoRoute(
        path: '/profile/edit',
        builder: (_, __) => const ProfileSetupScreen(editing: true),
      ),
      GoRoute(
        path: '/privacy/blocked',
        builder: (_, __) => const BlockedUsersScreen(),
      ),
      GoRoute(path: '/chats/new', builder: (_, __) => const NewChatScreen()),
      GoRoute(
        path: '/chat/:chatId',
        builder: (_, state) => ChatScreen(
          chatId: state.pathParameters['chatId']!,
          title: state.uri.queryParameters['title'] ?? 'Chat',
          isGroup: state.uri.queryParameters['group'] == 'true',
          peerUserId: state.uri.queryParameters['peer'],
        ),
      ),
      GoRoute(
        path: '/groups/create',
        builder: (_, __) => const CreateGroupScreen(),
      ),
      GoRoute(
        path: '/groups/join',
        builder: (_, state) => JoinGroupScreen(
          initialCode: state.uri.queryParameters['code'] ?? '',
        ),
      ),
      GoRoute(
        path: '/groups/:chatId',
        builder: (_, state) =>
            GroupInfoScreen(chatId: state.pathParameters['chatId']!),
      ),
    ],
  );
  ref.onDispose(() {
    router.dispose();
    refresh.dispose();
  });
  return router;
});
