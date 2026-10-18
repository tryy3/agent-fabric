import 'package:acpd/acpd.dart' show AvailableCommand, AvailableCommandsUpdate;

/// A slash command the plane advertises for a session: an enabled skill the
/// user can start with `/name` (ACP `available_commands_update`).
class SlashCommand {
  const SlashCommand({
    required this.name,
    required this.description,
    this.hint,
  });

  /// The command without the leading slash.
  final String name;
  final String description;

  /// What to type after the name, for example `<question>`.
  final String? hint;

  /// The text the composer inserts for this command.
  String get insertText => '/$name ';
}

/// Maps an ACP available-commands update to [SlashCommand]s, sorted by name.
List<SlashCommand> slashCommandsFrom(AvailableCommandsUpdate update) {
  SlashCommand one(AvailableCommand c) {
    final hint = c.input?.hint;
    return SlashCommand(
      name: c.name,
      description: c.description,
      hint: hint == null || hint.isEmpty ? null : hint,
    );
  }

  return [for (final c in update.availableCommands) one(c)]
    ..sort((a, b) => a.name.compareTo(b.name));
}

/// The commands that match what the user has typed after the leading slash.
/// Returns an empty list when [input] is not a bare `/name` being typed.
List<SlashCommand> matchSlashCommands(
  List<SlashCommand> commands,
  String input,
) {
  if (!input.startsWith('/') || input.contains(RegExp(r'\s'))) {
    return const [];
  }
  final typed = input.substring(1).toLowerCase();
  return [
    for (final c in commands)
      if (c.name.toLowerCase().startsWith(typed)) c,
  ];
}

/// A session that advertises slash commands. Kept apart from AgentSessionApi
/// so fakes that do not need commands stay small.
abstract interface class SlashCommandApi {
  /// The commands the plane advertised for the current session; empty before
  /// a session starts or when no skill is enabled.
  List<SlashCommand> get slashCommands;

  /// Fires when [slashCommands] changed.
  Stream<void> get slashCommandsChanged;
}
