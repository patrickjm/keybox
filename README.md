# keybox

Store reusable env var sets in macOS Keychain and emit .env files on demand.

## Install (local)

```bash
go install ./...
```

## Install (Homebrew)

```bash
brew install patrickjm/tap/keybox
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

# Rename keys on output
keybox env aws/dev --rename AWS_ACCESS_KEY_ID=AWS_KEY_ID -o .env

# Append without overwrite checks
keybox env aws/dev -o .env --append

# Overwrite existing keys only with confirmation
keybox env aws/dev -o .env --confirm-overwrite

# List sets and keys
keybox list
keybox keys aws/dev
```

## Notes

- Stores all sets in a single Keychain item (service `keybox`, account `store`).
- Configure with `--service` and `--account` if you want a different item.
