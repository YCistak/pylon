// Command pylon-sign generates the release signing key and signs release
// checksums with it. It is a development/CI tool and is not shipped.
//
// The private key must never travel further than the machine that made it and
// the CI secret it is pasted into:
//
//	go run ./cmd/pylon-sign keygen
//
// Paste the private half into the repo's RELEASE_SIGNING_KEY secret and the
// public half into selfupdate.publicKey. Then CI signs each release:
//
//	go run ./cmd/pylon-sign sign SHA256SUMS   # writes SHA256SUMS.sig
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

const keyEnv = "RELEASE_SIGNING_KEY"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen()
	case "sign":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		err = sign(os.Args[2])
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `pylon-sign — release signing (dev/CI tool)

usage:
  pylon-sign keygen        print a fresh keypair
  pylon-sign sign <file>   sign <file>, writing <file>.sig (key from $%s)
`, keyEnv)
}

// keygen prints a new keypair. Generating it here, rather than anywhere the
// private half could be logged or pasted into a transcript, is the point: the
// key that signs releases should exist only on the machine that made it.
func keygen() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	fmt.Printf(`public key (commit this — internal/selfupdate/selfupdate.go, publicKey):

    %s

private key (paste into the %s repository secret, then forget it —
it is not recoverable and must not be committed, logged, or shared):

    %s

`, base64.StdEncoding.EncodeToString(pub), keyEnv, base64.StdEncoding.EncodeToString(priv))
	return nil
}

func sign(path string) error {
	raw := os.Getenv(keyEnv)
	if raw == "" {
		return fmt.Errorf("$%s is empty", keyEnv)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return fmt.Errorf("decode $%s: %w", keyEnv, err)
	}
	if len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("$%s is %d bytes, want %d", keyEnv, len(key), ed25519.PrivateKeySize)
	}

	msg, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(ed25519.PrivateKey(key), msg)
	out := path + ".sig"
	if err := os.WriteFile(out, []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}
