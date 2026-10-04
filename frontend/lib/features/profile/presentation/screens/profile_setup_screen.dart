import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../auth/presentation/providers/auth_providers.dart';

final profileRemoteUpdateProvider = Provider(
  (ref) => _ProfileUpdater(ref.read(dioProvider)),
);

class _ProfileUpdater {
  final Dio _dio;
  _ProfileUpdater(this._dio);

  Future<void> updateProfile({
    required String displayName,
    String? username,
  }) async {
    await _dio.patch(
      '/users/me',
      data: {
        'displayName': displayName,
        if (username != null && username.isNotEmpty) 'username': username,
      },
    );
  }
}

class ProfileSetupScreen extends ConsumerStatefulWidget {
  final bool editing;
  const ProfileSetupScreen({super.key, this.editing = false});

  @override
  ConsumerState<ProfileSetupScreen> createState() => _ProfileSetupScreenState();
}

class _ProfileSetupScreenState extends ConsumerState<ProfileSetupScreen> {
  final _formKey = GlobalKey<FormState>();
  final _displayNameController = TextEditingController();
  final _usernameController = TextEditingController();
  bool _isSaving = false;
  bool _isLoading = false;
  bool _loadFailed = false;
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    if (widget.editing) _load();
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadFailed = false;
      _errorMessage = null;
    });
    try {
      final response = await ref.read(dioProvider).get('/users/me');
      if (!mounted) return;
      final data = response.data as Map<String, dynamic>;
      _displayNameController.text = data['displayName'] as String;
      _usernameController.text = data['username'] as String? ?? '';
    } catch (_) {
      if (mounted) {
        setState(() {
          _loadFailed = true;
          _errorMessage = 'Could not load profile';
        });
      }
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() {
      _isSaving = true;
      _errorMessage = null;
    });

    try {
      await ref
          .read(profileRemoteUpdateProvider)
          .updateProfile(
            displayName: _displayNameController.text.trim(),
            username: _usernameController.text.trim().isEmpty
                ? null
                : _usernameController.text.trim(),
          );

      if (!mounted) return;
      if (widget.editing) {
        context.pop();
      } else {
        ref.read(authControllerProvider.notifier).completeProfileSetup();
        context.go('/home');
      }
    } on DioException catch (e) {
      if (!mounted) return;
      final message = (e.response?.data is Map)
          ? (e.response!.data['message'] as String?)
          : null;
      setState(
        () => _errorMessage =
            message ?? 'Could not save your profile. Please try again.',
      );
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }

  @override
  void dispose() {
    _displayNameController.dispose();
    _usernameController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.editing ? 'Edit profile' : 'Set up your profile'),
        automaticallyImplyLeading: widget.editing,
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Form(
            key: _formKey,
            child: ListView(
              children: [
                if (_isLoading) const LinearProgressIndicator(),
                if (_loadFailed)
                  TextButton.icon(
                    onPressed: _load,
                    icon: const Icon(Icons.refresh),
                    label: const Text('Retry'),
                  ),
                const CircleAvatar(
                  radius: 48,
                  child: Icon(Icons.person, size: 48),
                ),
                const SizedBox(height: 24),
                TextFormField(
                  controller: _displayNameController,
                  enabled: !_isLoading && !_isSaving && !_loadFailed,
                  maxLength: 80,
                  decoration: const InputDecoration(
                    labelText: 'Display name',
                    border: OutlineInputBorder(),
                  ),
                  validator: (v) => (v == null || v.trim().isEmpty)
                      ? 'Display name is required'
                      : null,
                ),
                const SizedBox(height: 16),
                TextFormField(
                  controller: _usernameController,
                  enabled: !_isLoading && !_isSaving && !_loadFailed,
                  decoration: const InputDecoration(
                    labelText: 'Username (optional)',
                    border: OutlineInputBorder(),
                    prefixText: '@',
                  ),
                  validator: (v) {
                    if (v == null || v.isEmpty) return null;
                    if (v.length < 3 || v.length > 30) {
                      return 'Username must be 3-30 characters';
                    }
                    if (!RegExp(r'^[a-zA-Z0-9]+$').hasMatch(v)) {
                      return 'Only letters and numbers';
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 24),
                if (_errorMessage != null)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: Text(
                      _errorMessage!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                FilledButton(
                  onPressed: _isSaving || _isLoading || _loadFailed
                      ? null
                      : _submit,
                  child: _isSaving
                      ? const SizedBox(
                          height: 20,
                          width: 20,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : Text(widget.editing ? 'Save' : 'Continue'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
