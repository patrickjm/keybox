# keybox

Store reusable env var sets in macOS Keychain and emit .env files on demand.

## Install (local)

```bash
go install ./...
```

## Usage

```bash
# Store values, using env fallback for keys without explicit values
export OPENAI_API_KEY=sk-...
keybox set openai OPENAI_API_KEY

# Store explicit values
keybox set aws/dev AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=...

# Emit a .env payload (multiple sets merge, later overrides earlier)
keybox env aws/dev openai -o .env

# List sets and keys
keybox list
keybox keys aws/dev
```

## Notes

- Stores all sets in a single Keychain item (service `keybox`, account `store`).
- Configure with `--service` and `--account` if you want a different item.
