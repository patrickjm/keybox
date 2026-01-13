package main

import (
	"fmt"
	"os/exec"
	"strings"
)

type securityKeychain struct{}

func (securityKeychain) Read(service, account string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-a", account, "-s", service, "-w")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		msg := string(exitErr.Stderr)
		if strings.Contains(msg, "could not be found") {
			return "", errKeychainNotFound
		}
		return "", fmt.Errorf("keychain read failed: %s", strings.TrimSpace(msg))
	}
	return "", err
}

func (securityKeychain) Write(service, account, payload string) error {
	cmd := exec.Command("security", "add-generic-password", "-a", account, "-s", service, "-w", payload, "-U")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("keychain write failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
