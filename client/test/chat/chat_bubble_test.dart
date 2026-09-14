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
}
