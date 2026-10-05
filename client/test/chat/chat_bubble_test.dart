import 'package:agent_fabric_client/chat/cost_format.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final created = DateTime.utc(2026, 9, 14);

  test('user row maps to one user bubble', () {
    final bubbles = bubblesFromThreadMessage(
      ThreadMessage(
        id: 'm1',
        role: 'user',
        content: 'hi',
        position: 0,
        createdAt: created,
      ),
    );
    expect(bubbles.map((b) => b.kind).toList(), [ChatBubbleKind.user]);
    expect(bubbles.single.text, 'hi');
    expect(bubbles.single.createdAt, created);
  });

  test(
    'a divider with the round cost precedes the tool calls of each round',
    () {
      final bubbles = bubblesFromThreadMessage(
        ThreadMessage(
          id: 'm3',
          role: 'assistant',
          content: 'done',
          position: 1,
          createdAt: created,
          usage: const TurnUsage(
            rounds: [
              {
                'round': 0,
                'promptTokens': 1200,
                'completionTokens': 300,
                'cost': {'total': 0.0012},
              },
              {'round': 1, 'promptTokens': 1600},
            ],
          ),
          activities: const [
            TurnActivity.toolCall(
              ThreadToolCall(id: 'a', title: 'a', round: 0),
            ),
            TurnActivity.toolCall(
              ThreadToolCall(id: 'b', title: 'b', round: 0),
            ),
            TurnActivity.toolCall(
              ThreadToolCall(id: 'c', title: 'c', round: 1),
            ),
          ],
        ),
      );
      expect(bubbles.map((b) => b.kind).toList(), [
        ChatBubbleKind.roundCost,
        ChatBubbleKind.toolCall,
        ChatBubbleKind.toolCall,
        ChatBubbleKind.roundCost,
        ChatBubbleKind.toolCall,
        ChatBubbleKind.message,
        ChatBubbleKind.stats,
      ]);
      expect(
        roundCostLabel(bubbles.first.usage!),
        r'Round 1 · 1.2K in · 300 out · ~$0.0012',
      );
    },
  );

  test('assistant thought then content then usage maps in that order', () {
    final bubbles = bubblesFromThreadMessage(
      ThreadMessage(
        id: 'm2',
        role: 'assistant',
        content: 'hello',
        position: 1,
        createdAt: created,
        thought: 'hmm',
        model: 'm1',
        providerName: 'Local',
        stopReason: 'end_turn',
        usage: const TurnUsage(predictedPerSecond: 35.5, deltas: 1),
      ),
    );
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.thought,
      ChatBubbleKind.message,
      ChatBubbleKind.stats,
    ]);
    expect(bubbles[0].text, 'hmm');
    expect(bubbles[1].text, 'hello');
    expect(bubbles[1].model, 'm1');
    expect(bubbles[1].providerName, 'Local');
    expect(bubbles[1].predictedPerSecond, 35.5);
    expect(bubbles[2].usage?.deltas, 1);
    expect(bubbles[2].stopReason, 'end_turn');
    expect(
      bubbles.firstWhere((b) => b.kind == ChatBubbleKind.message).createdAt,
      created,
    );
  });

  test('assistant sent part maps to prompt bubble before thought', () {
    final message = ThreadMessage.fromJson({
      'id': 'm-sent',
      'role': 'assistant',
      'content': 'hello',
      'position': 1,
      'createdAt': '2026-09-14T00:00:00Z',
      'parts': [
        {
          'type': 'sent',
          'text': '<platform_instructions>\nABC\n</platform_instructions>\n\n<assistant_instructions>\n123\n</assistant_instructions>\n\n<runtime_context>\nDate\n</runtime_context>',
        },
        {'type': 'thought', 'text': 'hmm'},
        {'type': 'message', 'text': 'hello'},
      ],
    });
    final bubbles = bubblesFromThreadMessages([
      ThreadMessage(
        id: 'u1',
        role: 'user',
        content: 'hi',
        position: 0,
        createdAt: created,
      ),
      message,
    ]);
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.sent,
      ChatBubbleKind.user,
      ChatBubbleKind.thought,
      ChatBubbleKind.message,
    ]);
    expect(bubbles[0].text, contains('<platform_instructions>'));
    expect(bubbles[0].text, contains('<runtime_context>'));
    expect(bubbles[0].text, isNot(contains('## Messages')));
  });

  test('assistant content only is a single message bubble', () {
    final bubbles = bubblesFromThreadMessage(
      ThreadMessage(
        id: 'm3',
        role: 'assistant',
        content: 'hello',
        position: 1,
        createdAt: created,
      ),
    );
    expect(bubbles.map((b) => b.kind).toList(), [ChatBubbleKind.message]);
  });

  test('assistant tool-call JSON parts hydrate in order before message', () {
    final message = ThreadMessage.fromJson({
      'id': 'm4',
      'role': 'assistant',
      'content': 'done',
      'position': 1,
      'createdAt': created.toIso8601String(),
      'parts': [
        {
          'type': 'tool_call',
          'toolCallId': 'call_1',
          'title': 'Read file',
          'status': 'completed',
          'input': '{"path":"notes.txt"}',
          'output': '{}',
        },
        {
          'type': 'tool_call',
          'toolCallId': 'call_2',
          'title': 'List directory',
          'status': 'failed',
          'input': '{"path":"."}',
          'output': 'denied',
        },
        {'type': 'message', 'text': 'done'},
      ],
    });

    final bubbles = bubblesFromThreadMessage(message);
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.toolCall,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.message,
    ]);
    expect(bubbles[0].toolCallId, 'call_1');
    expect(bubbles[0].toolTitle, 'Read file');
    expect(bubbles[0].toolStatus, 'completed');
    expect(bubbles[0].streamingTool, isFalse);
    expect(bubbles[1].toolCallId, 'call_2');
    expect(bubbles[1].toolTitle, 'List directory');
    expect(bubbles[1].toolStatus, 'failed');
    expect(bubbles[1].streamingTool, isFalse);
    expect(bubbles[2].text, 'done');
  });

  test('interleaved thought and tool parts keep event order after hydrate', () {
    final message = ThreadMessage.fromJson({
      'id': 'm6',
      'role': 'assistant',
      'content': 'done',
      'position': 1,
      'createdAt': created.toIso8601String(),
      'parts': [
        {'type': 'thought', 'text': 'first'},
        {
          'type': 'tool_call',
          'toolCallId': 'call_1',
          'title': 'Read file',
          'status': 'completed',
          'input': '{"path":"a"}',
          'output': '{}',
        },
        {'type': 'thought', 'text': 'second'},
        {
          'type': 'tool_call',
          'toolCallId': 'call_2',
          'title': 'Read file',
          'status': 'completed',
          'input': '{"path":"b"}',
          'output': '{}',
        },
        {'type': 'thought', 'text': 'third'},
        {'type': 'message', 'text': 'done'},
      ],
    });

    final bubbles = bubblesFromThreadMessage(message);
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.thought,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.thought,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.thought,
      ChatBubbleKind.message,
    ]);
    expect(bubbles[0].text, 'first');
    expect(bubbles[1].toolCallId, 'call_1');
    expect(bubbles[2].text, 'second');
    expect(bubbles[3].toolCallId, 'call_2');
    expect(bubbles[4].text, 'third');
    expect(bubbles[5].text, 'done');
  });

  test('hydrated tool calls with missing or unknown status stay streaming', () {
    final message = ThreadMessage.fromJson({
      'id': 'm5',
      'role': 'assistant',
      'content': 'working',
      'position': 1,
      'createdAt': created.toIso8601String(),
      'parts': [
        {
          'type': 'tool_call',
          'toolCallId': 'call_missing',
          'name': 'read_file',
          'title': 'Read file',
          'input': '{"path":"notes.txt"}',
        },
        {
          'type': 'tool_call',
          'toolCallId': 'call_unknown',
          'name': 'list_directory',
          'title': 'List directory',
          'status': 'in_progress',
        },
      ],
    });

    final bubbles = bubblesFromThreadMessage(message);
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.toolCall,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.message,
    ]);
    expect(bubbles[0].streamingTool, isTrue);
    expect(bubbles[1].streamingTool, isTrue);
  });

  test('bubbleCaption joins model provider tok/s', () {
    expect(
      bubbleCaption(
        const ChatBubble(
          kind: ChatBubbleKind.message,
          text: 'hello',
          model: 'm1',
          providerName: 'Local',
          predictedPerSecond: 12.5,
        ),
      ),
      'm1 - Local - 12.5 tok/s',
    );
    expect(
      bubbleCaption(const ChatBubble(kind: ChatBubbleKind.thought, text: 'x')),
      '',
    );
  });

  test('bubbleCaption rounds tok/s to at most two decimals', () {
    expect(
      bubbleCaption(
        const ChatBubble(
          kind: ChatBubbleKind.message,
          text: 'hello',
          predictedPerSecond: 10.0,
        ),
      ),
      '10 tok/s',
    );
  });

  test('activityDescription uses first non-empty line', () {
    expect(activityDescription('hmm\nmore'), 'hmm');
    expect(activityDescription('\n  plan a story  \nrest'), 'plan a story');
    expect(activityDescription(''), '');
    expect(activityDescription('single'), 'single');
  });

  test(
    'failed attempt with error part hydrates requestFailed after message',
    () {
      final message = ThreadMessage.fromJson({
        'id': 'm-fail',
        'role': 'assistant',
        'content': 'partial',
        'position': 1,
        'createdAt': created.toIso8601String(),
        'status': 'failed',
        'stopReason': 'error',
        'parts': [
          {'type': 'thought', 'text': 'hmm'},
          {'type': 'message', 'text': 'partial'},
          {
            'type': 'error',
            'text': 'Inference failed (Local): OpenAI HTTP 502: overloaded',
            'status': 'failed',
          },
        ],
      });
      final bubbles = bubblesFromThreadMessage(message);
      expect(bubbles.map((b) => b.kind).toList(), [
        ChatBubbleKind.thought,
        ChatBubbleKind.message,
        ChatBubbleKind.requestFailed,
        ChatBubbleKind.stats,
      ]);
      expect(bubbles[2].text, contains('overloaded'));
    },
  );

  test('failed attempt without error part synthesizes requestFailed', () {
    final bubbles = bubblesFromThreadMessage(
      ThreadMessage(
        id: 'm-int',
        role: 'assistant',
        content: '',
        position: 1,
        createdAt: created,
        status: 'failed',
        stopReason: 'interrupted',
      ),
    );
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.message,
      ChatBubbleKind.requestFailed,
      ChatBubbleKind.stats,
    ]);
    expect(bubbles[1].text, 'Inference failed: interrupted');
  });

  test('failed retry fork tip keeps partials; successful retry hides earlier failure', () {
    final user = ThreadMessage.fromJson({
      'id': 'm-user',
      'role': 'user',
      'content': 'hi',
      'position': 0,
      'createdAt': created.toIso8601String(),
    });
    final prior = ThreadMessage.fromJson({
      'id': 'm-prior',
      'role': 'assistant',
      'content': 'old answer',
      'position': 1,
      'createdAt': created.toIso8601String(),
      'active': true,
      'status': 'completed',
      'promptMessageId': 'm-user',
      'parts': [
        {'type': 'message', 'text': 'old answer'},
      ],
    });
    final draft = ThreadMessage.fromJson({
      'id': 'm-draft',
      'role': 'assistant',
      'content': '',
      'position': 2,
      'createdAt': created.toIso8601String(),
      'active': false,
      'status': 'failed',
      'stopReason': 'error',
      'promptMessageId': 'm-user',
      'parts': [
        {'type': 'thought', 'text': 'planning'},
        {
          'type': 'tool_call',
          'toolCallId': 'call_1',
          'title': 'Read file',
          'status': 'completed',
          'input': '{"path":"a"}',
          'output': '{}',
        },
        {
          'type': 'error',
          'text': 'Inference failed (Local): dial tcp 127.0.0.1:8888: connection refused',
          'status': 'failed',
        },
      ],
    });

    expect(bubblesFromThreadMessage(draft), isEmpty);

    final failedFork = bubblesFromThreadMessages([user, prior, draft]);
    expect(failedFork.map((b) => b.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.thought,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.requestFailed,
      ChatBubbleKind.stats,
    ]);
    expect(failedFork[1].text, 'planning');
    expect(failedFork[3].text, contains('connection refused'));

    final priorInactive = ThreadMessage.fromJson({
      'id': 'm-prior',
      'role': 'assistant',
      'content': 'old answer',
      'position': 1,
      'createdAt': created.toIso8601String(),
      'active': false,
      'status': 'completed',
      'promptMessageId': 'm-user',
      'parts': [
        {'type': 'message', 'text': 'old answer'},
      ],
    });
    final success = ThreadMessage.fromJson({
      'id': 'm-new',
      'role': 'assistant',
      'content': 'new answer',
      'position': 3,
      'createdAt': created.toIso8601String(),
      'active': true,
      'status': 'completed',
      'promptMessageId': 'm-user',
      'parts': [
        {'type': 'message', 'text': 'new answer'},
      ],
    });
    final successFork = bubblesFromThreadMessages([
      user,
      priorInactive,
      draft,
      success,
    ]);
    expect(successFork.map((b) => b.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.message,
    ]);
    expect(successFork[1].text, 'new answer');
  });

  test('active failed mid-turn keeps thought and tool before error', () {
    final user = ThreadMessage.fromJson({
      'id': 'm-user',
      'role': 'user',
      'content': 'hi',
      'position': 0,
      'createdAt': created.toIso8601String(),
    });
    final failed = ThreadMessage.fromJson({
      'id': 'm-as',
      'role': 'assistant',
      'content': '',
      'position': 1,
      'createdAt': created.toIso8601String(),
      'active': true,
      'status': 'failed',
      'stopReason': 'error',
      'promptMessageId': 'm-user',
      'parts': [
        {'type': 'thought', 'text': 'first plan'},
        {
          'type': 'tool_call',
          'toolCallId': 'call_1',
          'title': 'web_search',
          'status': 'completed',
          'input': '{"q":"x"}',
          'output': '[]',
        },
        {
          'type': 'error',
          'text': 'Inference failed (Local): connection refused',
          'status': 'failed',
        },
      ],
    });
    final bubbles = bubblesFromThreadMessages([user, failed]);
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.thought,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.requestFailed,
      ChatBubbleKind.stats,
    ]);
  });
}
