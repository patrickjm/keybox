package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultService = "keybox"
	defaultAccount = "store"
)

type store map[string]map[string]string

type config struct {
	service string
	account string
	output  string
	asJSON  bool
	append  bool
	confirm bool
	renames []string
}

type app struct {
	keychain Keychain
	cfg      config
}

func NewRootCmd(keychain Keychain, version string) *cobra.Command {
	app := &app{
		keychain: keychain,
		cfg: config{
			service: defaultService,
			account: defaultAccount,
		},
	}

	rootCmd := &cobra.Command{
		Use:   "keybox",
		Short: "Store and emit reusable env var sets from macOS Keychain",
	}
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("keybox {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&app.cfg.service, "service", defaultService, "Keychain service name")
	rootCmd.PersistentFlags().StringVar(&app.cfg.account, "account", defaultAccount, "Keychain account name")
	rootCmd.PersistentFlags().BoolVar(&app.cfg.asJSON, "json", false, "Output JSON when supported")

	rootCmd.AddCommand(app.newSetCmd())
	rootCmd.AddCommand(app.newGetCmd())
	rootCmd.AddCommand(app.newEnvCmd())
	rootCmd.AddCommand(app.newListCmd())
	rootCmd.AddCommand(app.newKeysCmd())
	rootCmd.AddCommand(app.newRmCmd())

	return rootCmd
}

func (app *app) newSetCmd() *cobra.Command {
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

			st, err := app.loadStore()
			if err != nil {
				return err
			}
			if st[setName] == nil {
				st[setName] = map[string]string{}
			}
			for k, v := range updates {
				st[setName][k] = v
			}
			if err := app.saveStore(st); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %s (%d keys)\n", setName, len(updates))
			return nil
		},
	}
	return cmd
}

func (app *app) newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <set> [KEY...]",
		Short: "Get values from a set",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			st, err := app.loadStore()
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
			if app.cfg.asJSON {
				return writeJSON(cmd, out)
			}
			writeKeyValues(cmd, keys, out)
			return nil
		},
	}
	return cmd
}

func (app *app) newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env <set> [<set>...]",
		Short: "Emit a .env payload for one or more sets",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := app.loadStore()
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

			if len(app.cfg.renames) > 0 {
				renamed, err := applyRenames(merged, app.cfg.renames)
				if err != nil {
					return err
				}
				merged = renamed
			}

			if app.cfg.asJSON {
				return writeJSON(cmd, merged)
			}
			out := cmd.OutOrStdout()
			keys := sortedKeys(merged)
			if app.cfg.output != "" {
				if app.cfg.append {
					file, err := os.OpenFile(app.cfg.output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
					if err != nil {
						return err
					}
					defer file.Close()
					out = file
				} else {
					if err := app.ensureNoOverwrite(keys); err != nil {
						return err
					}
					file, err := os.Create(app.cfg.output)
					if err != nil {
						return err
					}
					defer file.Close()
					out = file
				}
			} else if app.cfg.append {
				return errors.New("--append requires --output")
			}
			for _, key := range keys {
				fmt.Fprintf(out, "%s=%s\n", key, merged[key])
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&app.cfg.output, "output", "o", "", "Write output to a file instead of stdout")
	cmd.Flags().BoolVar(&app.cfg.append, "append", false, "Append to the output file instead of replacing it")
	cmd.Flags().BoolVar(&app.cfg.confirm, "confirm-overwrite", false, "Allow overwriting existing keys in the output file")
	cmd.Flags().StringArrayVar(&app.cfg.renames, "rename", nil, "Rename keys as OLD=NEW (repeatable)")
	return cmd
}

func (app *app) newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available sets",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := app.loadStore()
			if err != nil {
				return err
			}
			sets := sortedKeys(st)
			if app.cfg.asJSON {
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

func (app *app) newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys <set>",
		Short: "List keys in a set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			st, err := app.loadStore()
			if err != nil {
				return err
			}
			values, ok := st[setName]
			if !ok {
				return fmt.Errorf("set %q not found", setName)
			}
			keys := sortedKeys(values)
			if app.cfg.asJSON {
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

func (app *app) newRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <set> [KEY...]",
		Short: "Remove keys or whole set",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			st, err := app.loadStore()
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
				if err := app.saveStore(st); err != nil {
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
			if err := app.saveStore(st); err != nil {
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

func (app *app) loadStore() (store, error) {
	data, err := app.keychain.Read(app.cfg.service, app.cfg.account)
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

func (app *app) saveStore(st store) error {
	payload, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return app.keychain.Write(app.cfg.service, app.cfg.account, string(payload))
}

func (app *app) ensureNoOverwrite(keys []string) error {
	if app.cfg.confirm || app.cfg.output == "" {
		return nil
	}
	existing, err := readEnvKeys(app.cfg.output)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var conflicts []string
	for _, key := range keys {
		if _, ok := existing[key]; ok {
			conflicts = append(conflicts, key)
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("refusing to overwrite %d key(s) in %s (use --confirm-overwrite): %s", len(conflicts), app.cfg.output, strings.Join(conflicts, ", "))
}

func applyRenames(values map[string]string, renames []string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range values {
		out[k] = v
	}
	for _, rename := range renames {
		parts := strings.SplitN(rename, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid rename %q, expected OLD=NEW", rename)
		}
		oldKey := strings.TrimSpace(parts[0])
		newKey := strings.TrimSpace(parts[1])
		val, ok := out[oldKey]
		if !ok {
			return nil, fmt.Errorf("rename source %q not found", oldKey)
		}
		if _, ok := out[newKey]; ok && oldKey != newKey {
			return nil, fmt.Errorf("rename target %q already exists", newKey)
		}
		delete(out, oldKey)
		out[newKey] = val
	}
	return out, nil
}

func readEnvKeys(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	keys := map[string]struct{}{}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		keys[key] = struct{}{}
	}
	return keys, nil
}
