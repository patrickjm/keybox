---
name: keybox
description: Use when storing or emitting reusable env var sets via the keybox CLI, including set/get/env/list/keys/rm workflows, rename-on-output, and non-interactive overwrite protection.
---

# Keybox CLI

## Quick workflow

1) Store values in a named set.
2) Emit `.env` output from one or more sets.
3) Clean up with `rm` when done.

## Store values

Use `KEY=VALUE` pairs. When only `KEY` is provided, keybox reads the value from the current shell environment.

```bash
keybox set openai OPENAI_API_KEY
keybox set aws/dev AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=...
```

## Emit .env

Emit one or more sets. Later sets override earlier ones.

```bash
keybox env aws/dev openai -o .env
```

### Rename on output

Use `--rename OLD=NEW` (repeatable) to rename keys as they are emitted.

```bash
keybox env aws/dev --rename AWS_ACCESS_KEY_ID=AWS_KEY_ID -o .env
```

### Non-interactive overwrite protection

Writing to an existing `.env` refuses to overwrite existing keys unless `--confirm-overwrite` is set.

```bash
keybox env aws/dev -o .env --confirm-overwrite
```

To avoid checks and only append, use `--append` (requires `--output`):

```bash
keybox env aws/dev -o .env --append
```

## Inspect and remove

```bash
keybox list
keybox keys aws/dev
keybox get aws/dev AWS_ACCESS_KEY_ID
keybox rm aws/dev AWS_ACCESS_KEY_ID
keybox rm aws/dev
```

## Configuration

Use `--service` and `--account` to change the Keychain item name.
