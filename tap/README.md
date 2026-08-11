# Catena Tap

Catena Tap runs one supported Agent Runtime through the pinned
[`claude-tap`](https://github.com/liaohch3/claude-tap) capture engine, rebuilds
each user turn as `Turn → Model → Tool` and uploads it to Catena with an
Agent-bound API key.

## Install

```bash
cd tap
python3 -m pip install .
```

## Connect an Agent

Copy the Agent key from Catena's **API management** page, then run:

```bash
export CATENA_URL="https://your-catena.example"
export CATENA_API_KEY="catena_agent_..."

catena tap codex
catena tap claude
catena tap hermes
catena tap openclaw
```

Arguments after `--` are passed to the target Runtime:

```bash
catena tap codex -- --full-auto
catena tap claude -- -p "inspect this repository"
```

`catena tap codexapp` launches a separately proxied Codex/ChatGPT desktop
instance using claude-tap's existing forward-proxy support.

The upstream local viewer is disabled by default because Catena is the product
UI. Pass `--tap-ui` to keep it during connector debugging.

## Failure behavior

Capture and upload fail open. A network error may lose a Trace, but it cannot
change the target Agent's response or exit code. An unfinished buffered turn is
uploaded during normal shutdown with `catena.turn.incomplete=true`.
