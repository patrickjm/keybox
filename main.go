package main

import "os"

var version = "dev"

func main() {
	rootCmd := NewRootCmd(securityKeychain{}, version)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
