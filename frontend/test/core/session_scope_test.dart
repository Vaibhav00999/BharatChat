import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:bharatchat/features/auth/presentation/providers/auth_providers.dart';
import 'package:bharatchat/features/chat/presentation/providers/chat_providers.dart';
import 'package:bharatchat/features/chat_list/presentation/providers/chat_list_providers.dart';
import '../features/auth/presentation/providers/auth_controller_test.dart'
    show FakeAuthRepository;
import '../features/chat/presentation/providers/chat_controller_test.dart'
    show FakeWsClient, FakeMessageRemoteDataSource;
import '../features/chat_list/presentation/providers/chat_list_controller_test.dart'
    show FakeChatRemoteDataSource;

class SessionController extends AuthController {
  SessionController() : super(FakeAuthRepository());
  void signInAs(String id) {
    state = AuthState(status: AuthStatus.authenticated, userId: id);
  }
}

void main() {
  test(
    'account changes replace chat controllers and their cached messages',
    () async {
      final auth = SessionController();
      final ws = FakeWsClient();
      final container = ProviderContainer(
        overrides: [
          authControllerProvider.overrideWith((ref) => auth),
          webSocketClientProvider.overrideWithValue(ws),
          messageRemoteDataSourceProvider.overrideWithValue(
            FakeMessageRemoteDataSource(),
          ),
          chatRemoteDataSourceProvider.overrideWithValue(
            FakeChatRemoteDataSource(),
          ),
        ],
      );
      addTearDown(() {
        container.dispose();
        ws.dispose();
      });
      await Future<void>.delayed(Duration.zero);
      auth.signInAs('alice');
      container.listen(chatControllerProvider('chat-1'), (_, __) {});
      container.listen(chatListProvider, (_, __) {});
      final oldChat = container.read(chatControllerProvider('chat-1').notifier);
      final oldList = container.read(chatListProvider.notifier);
      await Future<void>.delayed(Duration.zero);
      auth.signInAs('bob');
      final newChat = container.read(chatControllerProvider('chat-1').notifier);
      final newList = container.read(chatListProvider.notifier);
      expect(identical(oldChat, newChat), isFalse);
      expect(identical(oldList, newList), isFalse);
      expect(newChat.currentUserId, 'bob');
      expect(newChat.state.messages, isEmpty);
      await Future<void>.delayed(Duration.zero);
    },
  );
}
