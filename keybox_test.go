package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryKeychain struct {
	payload string
	exists  bool
}

func (m *memoryKeychain) Read(service, account string) (string, error) {
	if !m.exists {
		return "", errKeychainNotFound
	}
	return m.payload, nil
}

func (m *memoryKeychain) Write(service, account, payload string) error {
	m.payload = payload
	m.exists = true
	return nil
}

func runCmd(t *testing.T, keychain Keychain, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd(keychain, "test")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestSetEnvFallbackAndGet(t *testing.T) {
	keychain := &memoryKeychain{}
	t.Setenv("API_KEY", "abc123")

	if _, err := runCmd(t, keychain, "set", "demo", "API_KEY"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	out, err := runCmd(t, keychain, "get", "demo", "API_KEY")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if out != "API_KEY=abc123\n" {
		t.Fatalf("unexpected get output: %q", out)
	}

	listOut, err := runCmd(t, keychain, "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if strings.TrimSpace(listOut) != "demo" {
		t.Fatalf("unexpected list output: %q", listOut)
	}

	keysOut, err := runCmd(t, keychain, "keys", "demo")
	if err != nil {
		t.Fatalf("keys failed: %v", err)
	}
	if strings.TrimSpace(keysOut) != "API_KEY" {
		t.Fatalf("unexpected keys output: %q", keysOut)
	}
}

func TestSetExplicitAndGetAll(t *testing.T) {
	keychain := &memoryKeychain{}

	if _, err := runCmd(t, keychain, "set", "aws/dev", "AWS_ACCESS_KEY_ID=foo", "AWS_SECRET_ACCESS_KEY=bar"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	out, err := runCmd(t, keychain, "get", "aws/dev")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	expect := "AWS_ACCESS_KEY_ID=foo\nAWS_SECRET_ACCESS_KEY=bar\n"
	if out != expect {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestEnvMergeOverrides(t *testing.T) {
	keychain := &memoryKeychain{}

	if _, err := runCmd(t, keychain, "set", "base", "KEY=one", "OTHER=foo"); err != nil {
		t.Fatalf("set base failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "set", "override", "KEY=two"); err != nil {
		t.Fatalf("set override failed: %v", err)
	}

	out, err := runCmd(t, keychain, "env", "base", "override")
	if err != nil {
		t.Fatalf("env failed: %v", err)
	}
	expect := "KEY=two\nOTHER=foo\n"
	if out != expect {
		t.Fatalf("unexpected env output: %q", out)
	}
}

func TestEnvRename(t *testing.T) {
	keychain := &memoryKeychain{}

	if _, err := runCmd(t, keychain, "set", "demo", "OLD=1"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	out, err := runCmd(t, keychain, "env", "demo", "--rename", "OLD=NEW")
	if err != nil {
		t.Fatalf("env failed: %v", err)
	}
	if out != "NEW=1\n" {
		t.Fatalf("unexpected env output: %q", out)
	}
}

func TestEnvOverwriteRequiresConfirm(t *testing.T) {
	keychain := &memoryKeychain{}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	if err := os.WriteFile(path, []byte("KEY=old\n"), 0o600); err != nil {
		t.Fatalf("write env failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "set", "demo", "KEY=new"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "env", "demo", "-o", path); err == nil {
		t.Fatalf("expected overwrite error")
	}

	if _, err := runCmd(t, keychain, "env", "demo", "-o", path, "--confirm-overwrite"); err != nil {
		t.Fatalf("confirm overwrite failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	if string(data) != "KEY=new\n" {
		t.Fatalf("unexpected env contents: %q", string(data))
	}
}

func TestEnvAppendSkipsConfirm(t *testing.T) {
	keychain := &memoryKeychain{}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	if err := os.WriteFile(path, []byte("EXISTING=1\n"), 0o600); err != nil {
		t.Fatalf("write env failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "set", "demo", "NEW=2"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "env", "demo", "-o", path, "--append"); err != nil {
		t.Fatalf("append env failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "EXISTING=1\n") || !strings.Contains(out, "NEW=2\n") {
		t.Fatalf("unexpected appended contents: %q", out)
	}
}

func TestEnvAppendRequiresOutput(t *testing.T) {
	keychain := &memoryKeychain{}

	if _, err := runCmd(t, keychain, "set", "demo", "A=1"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "env", "demo", "--append"); err == nil {
		t.Fatalf("expected append error without output")
	}
}

func TestRemoveKeysAndSet(t *testing.T) {
	keychain := &memoryKeychain{}

	if _, err := runCmd(t, keychain, "set", "demo", "A=1", "B=2"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "rm", "demo", "B"); err != nil {
		t.Fatalf("rm key failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "get", "demo", "B"); err == nil {
		t.Fatalf("expected missing key error")
	}
	if _, err := runCmd(t, keychain, "rm", "demo"); err != nil {
		t.Fatalf("rm set failed: %v", err)
	}
	if _, err := runCmd(t, keychain, "get", "demo"); err == nil {
		t.Fatalf("expected missing set error")
	}
}

func TestMissingEnvVar(t *testing.T) {
	keychain := &memoryKeychain{}

	_, err := runCmd(t, keychain, "set", "demo", "MISSING")
	if err == nil {
		t.Fatalf("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "missing value for") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMissingSetError(t *testing.T) {
	keychain := &memoryKeychain{}

	_, err := runCmd(t, keychain, "get", "nope")
	if err == nil {
		t.Fatalf("expected error for missing set")
	}
	if !strings.Contains(err.Error(), "set \"nope\" not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJSONOutput(t *testing.T) {
	keychain := &memoryKeychain{}

	if _, err := runCmd(t, keychain, "set", "demo", "A=1"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	out, err := runCmd(t, keychain, "list", "--json")
	if err != nil {
		t.Fatalf("list json failed: %v", err)
	}
	if !strings.Contains(out, "\"demo\"") {
		t.Fatalf("unexpected json output: %q", out)
	}
}

func TestKeychainNotFound(t *testing.T) {
	keychain := &memoryKeychain{}

	out, err := runCmd(t, keychain, "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestWriteErrorPropagates(t *testing.T) {
	broken := &errorKeychain{err: errors.New("write failed")}

	cmd := NewRootCmd(broken, "test")
	cmd.SetArgs([]string{"set", "demo", "A=1"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write error, got %v", err)
	}
}

type errorKeychain struct {
	err error
}

func (e *errorKeychain) Read(service, account string) (string, error) {
	return "{}", nil
}

func (e *errorKeychain) Write(service, account, payload string) error {
	return e.err
}
