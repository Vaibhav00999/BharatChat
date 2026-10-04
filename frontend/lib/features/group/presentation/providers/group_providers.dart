import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../auth/presentation/providers/auth_providers.dart';
import '../../data/datasources/group_remote_datasource.dart';
import '../../domain/entities/group_info.dart';

final groupRemoteDataSourceProvider = Provider(
  (ref) => GroupRemoteDataSource(ref.read(dioProvider)),
);
const _keepGroupError = Object();

class GroupState {
  final GroupInfo? info;
  final List<GroupMember> members;
  final bool loading, saving;
  final String? error;
  const GroupState({
    this.info,
    this.members = const [],
    this.loading = true,
    this.saving = false,
    this.error,
  });
  GroupState copyWith({
    GroupInfo? info,
    List<GroupMember>? members,
    bool? loading,
    bool? saving,
    Object? error = _keepGroupError,
  }) => GroupState(
    info: info ?? this.info,
    members: members ?? this.members,
    loading: loading ?? this.loading,
    saving: saving ?? this.saving,
    error: identical(error, _keepGroupError) ? this.error : error as String?,
  );
}

class GroupController extends StateNotifier<GroupState> {
  final String id;
  final GroupRemoteDataSource remote;
  GroupController(this.id, this.remote) : super(const GroupState()) {
    refresh();
  }
  Future<void> refresh() async {
    state = state.copyWith(loading: true, error: null);
    try {
      final values = await Future.wait([
        remote.getInfo(id),
        remote.members(id),
      ]);
      state = GroupState(
        info: values[0] as GroupInfo,
        members: values[1] as List<GroupMember>,
        loading: false,
      );
    } catch (e) {
      state = state.copyWith(loading: false, error: 'Could not load group');
    }
  }

  Future<void> action(Future<void> Function() task) async {
    state = state.copyWith(saving: true, error: null);
    try {
      await task();
      await refresh();
    } catch (e) {
      state = state.copyWith(
        saving: false,
        error: 'The group could not be updated',
      );
    } finally {
      state = state.copyWith(saving: false);
    }
  }
}

final groupControllerProvider =
    StateNotifierProvider.family<GroupController, GroupState, String>(
      (ref, id) => GroupController(id, ref.read(groupRemoteDataSourceProvider)),
    );
