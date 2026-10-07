# ACP chat turn

## Sub-features
`initialize` → `session/new` (`_meta.assistantId`) → `session/prompt` with streamed `agent_message_chunk`.

## How to get to it (user POV)
Flutter chat, or `acp-cli`.

## Driving it with acp-cli
`.claude/skills/verify/drive-prompt.sh "hello verify"`.
Proof: stdout is `FAKE-REPLY: hello verify` and `$RUN/fake-llm.log` has `chat stream=true reply="FAKE-REPLY: hello verify"`.

## Gotchas
acp-cli has a 30s timeout and does not set `_meta.threadId`, so no thread row, messages or hop captures are persisted. Thread persistence needs the Flutter app or a client that sets the thread id. Permission requests are auto-cancelled by acp-cli.
