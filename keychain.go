package main

import "errors"

var errKeychainNotFound = errors.New("keychain item not found")

type Keychain interface {
	Read(service, account string) (string, error)
	Write(service, account, payload string) error
}
