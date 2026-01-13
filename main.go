package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultService = "keybox"
	defaultAccount = "store"
)

var (
	version = "dev"

	service string
	account string
	output  string
	asJSON  bool
)

type store map[string]map[string]string

func main() {
	rootCmd := &cobra.Command{
		Use:   "keybox",
		Short: "Store and emit reusable env var sets from macOS Keychain",
	}
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("keybox {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&service, "service", defaultService, "Keychain service name")
	rootCmd.PersistentFlags().StringVar(&account, "account", defaultAccount, "Keychain account name")
	rootCmd.PersistentFlags().BoolVar(&asJSON, "json", false, "Output JSON when supported")

	rootCmd.AddCommand(newSetCmd())
	rootCmd.AddCommand(newGetCmd())
	rootCmd.AddCommand(newEnvCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newKeysCmd())
	rootCmd.AddCommand(newRmCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <set> KEY[=VALUE]...",
		Short: "Store keys into a set",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			updates, err := parseKeyValues(args[1:])
			if err != nil {
				return err
			}

			st, err := loadStore()
			if err != nil {
				return err
			}
			if st[setName] == nil {
				st[setName] = map[string]string{}
			}
			for k, v := range updates {
				st[setName][k] = v
			}
			if err := saveStore(st); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %s (%d keys)\n", setName, len(updates))
			return nil
		},
	}
	return cmd
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <set> [KEY...]",
		Short: "Get values from a set",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			st, err := loadStore()
			if err != nil {
				return err
			}
			values, ok := st[setName]
			if !ok {
				return fmt.Errorf("set %q not found", setName)
			}

			keys := args[1:]
			if len(keys) == 0 {
				keys = sortedKeys(values)
			}
			out := map[string]string{}
			for _, key := range keys {
				val, ok := values[key]
				if !ok {
					return fmt.Errorf("key %q not found in set %q", key, setName)
				}
				out[key] = val
			}
			if asJSON {
				return writeJSON(cmd, out)
			}
			writeKeyValues(cmd, keys, out)
			return nil
		},
	}
	return cmd
}

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env <set> [<set>...]",
		Short: "Emit a .env payload for one or more sets",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadStore()
			if err != nil {
				return err
			}
			merged := map[string]string{}
			for _, setName := range args {
				values, ok := st[setName]
				if !ok {
					return fmt.Errorf("set %q not found", setName)
				}
				for k, v := range values {
					merged[k] = v
				}
			}
			if asJSON {
				return writeJSON(cmd, merged)
			}
			out := cmd.OutOrStdout()
			keys := sortedKeys(merged)
			if output != "" {
				file, err := os.Create(output)
				if err != nil {
					return err
				}
				defer file.Close()
				out = file
			}
			for _, key := range keys {
				fmt.Fprintf(out, "%s=%s\n", key, merged[key])
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write output to a file instead of stdout")
	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available sets",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadStore()
			if err != nil {
				return err
			}
			sets := sortedKeys(st)
			if asJSON {
				return writeJSON(cmd, sets)
			}
			for _, setName := range sets {
				fmt.Fprintln(cmd.OutOrStdout(), setName)
			}
			return nil
		},
	}
	return cmd
}

func newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys <set>",
		Short: "List keys in a set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			st, err := loadStore()
			if err != nil {
				return err
			}
			values, ok := st[setName]
			if !ok {
				return fmt.Errorf("set %q not found", setName)
			}
			keys := sortedKeys(values)
			if asJSON {
				return writeJSON(cmd, keys)
			}
			for _, key := range keys {
				fmt.Fprintln(cmd.OutOrStdout(), key)
			}
			return nil
		},
	}
	return cmd
}

func newRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <set> [KEY...]",
		Short: "Remove keys or whole set",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			st, err := loadStore()
			if err != nil {
				return err
			}
			values, ok := st[setName]
			if !ok {
				return fmt.Errorf("set %q not found", setName)
			}
			keys := args[1:]
			if len(keys) == 0 {
				delete(st, setName)
				if err := saveStore(st); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", setName)
				return nil
			}
			for _, key := range keys {
				if _, ok := values[key]; !ok {
					return fmt.Errorf("key %q not found in set %q", key, setName)
				}
				delete(values, key)
			}
			st[setName] = values
			if err := saveStore(st); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %d keys from %s\n", len(keys), setName)
			return nil
		},
	}
	return cmd
}

func parseKeyValues(args []string) (map[string]string, error) {
	out := map[string]string{}
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, errors.New("empty key")
		}
		if len(parts) == 2 {
			out[key] = parts[1]
			continue
		}
		val := os.Getenv(key)
		if val == "" {
			return nil, fmt.Errorf("missing value for %q and env var not set", key)
		}
		out[key] = val
	}
	return out, nil
}

func writeKeyValues(cmd *cobra.Command, keys []string, values map[string]string) {
	for _, key := range keys {
		fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, values[key])
	}
}

func writeJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func loadStore() (store, error) {
	data, err := readKeychain()
	if err != nil {
		if errors.Is(err, errKeychainNotFound) {
			return store{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(data) == "" {
		return store{}, nil
	}
	var st store
	if err := json.Unmarshal([]byte(data), &st); err != nil {
		return nil, fmt.Errorf("decode keybox store: %w", err)
	}
	return st, nil
}

func saveStore(st store) error {
	payload, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return writeKeychain(string(payload))
}

var errKeychainNotFound = errors.New("keychain item not found")

func readKeychain() (string, error) {
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

func writeKeychain(payload string) error {
	cmd := exec.Command("security", "add-generic-password", "-a", account, "-s", service, "-w", payload, "-U")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("keychain write failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
