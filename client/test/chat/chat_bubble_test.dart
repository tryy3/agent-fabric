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
  });

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
          'name': 'read_file',
          'title': 'Read file',
          'input': '{"path":"notes.txt"}',
          'output': '{"content":"hello"}',
          'status': 'completed',
          'text': 'Read notes.txt',
        },
        {
          'type': 'tool_call',
          'toolCallId': 'call_2',
          'name': 'list_directory',
          'title': 'List directory',
          'input': '{"path":"."}',
          'output': '["notes.txt"]',
          'status': 'failed',
        },
        {'type': 'message', 'text': 'done'},
      ],
    });

    final bubbles = bubblesFromThreadMessage(message);

    expect(bubbles.map((bubble) => bubble.kind), [
      ChatBubbleKind.toolCall,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.message,
    ]);
    expect(bubbles[0].toolCallId, 'call_1');
    expect(bubbles[0].toolTitle, 'Read file');
    expect(bubbles[0].toolStatus, 'completed');
    expect(bubbles[0].toolInput, '{"path":"notes.txt"}');
    expect(bubbles[0].toolOutput, '{"content":"hello"}');
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

  test(
    'hydrated tool calls with missing or unknown status stay streaming',
    () {
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

      expect(bubbles.map((bubble) => bubble.kind), [
        ChatBubbleKind.toolCall,
        ChatBubbleKind.toolCall,
        ChatBubbleKind.message,
      ]);
      expect(bubbles[0].toolCallId, 'call_missing');
      expect(bubbles[0].toolStatus, isNull);
      expect(bubbles[0].streamingTool, isTrue);
      expect(bubbles[1].toolCallId, 'call_unknown');
      expect(bubbles[1].toolStatus, 'in_progress');
      expect(bubbles[1].streamingTool, isTrue);
    },
  );

  test('bubbleCaption joins model provider tok/s', () {
    expect(
      bubbleCaption(
        const ChatBubble(
          kind: ChatBubbleKind.message,
          text: 'hello',
          model: 'm1',
          providerName: 'Local',
          predictedPerSecond: 35.5,
        ),
      ),
      'm1 · Local · 35.5 tok/s',
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
          model: 'm1',
          providerName: 'Local',
          predictedPerSecond: 192.14271380889564,
        ),
      ),
      'm1 · Local · 192.14 tok/s',
    );
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
}
