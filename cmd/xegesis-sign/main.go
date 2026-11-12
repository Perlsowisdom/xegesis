// Command xegesis-sign makes and uses the key that signs Xegesis updates.
//
//	xegesis-sign keygen [-out key.txt]          write a new key pair; print the public key
//	xegesis-sign sign -version 1.2.3 <file>     print the base64 signature for an installer
//	xegesis-sign hash <file>                    print the installer's SHA-256
//
// sign reads the private key from the XEGESIS_UPDATE_KEY environment variable (a secret in CI) or
// from the file named by -key. The private key is never written anywhere but the file keygen is
// told to write, and must never be committed (docs/releasing.md).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/update/updatesig"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "xegesis-sign:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New("usage: xegesis-sign keygen | sign -version V <file> | hash <file>")
	}
	switch args[0] {
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
		out := fs.String("out", "xegesis-update-key.txt", "file for the private key (refused if it exists)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		pub, priv, err := updatesig.GenerateKey()
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := f.WriteString(priv + "\n"); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Private key written to %s. Keep it secret and never commit it.\nPublic key (put this in internal/update/pubkey.txt):\n%s\n", *out, pub)
		return nil
	case "hash":
		if len(args) != 2 {
			return errors.New("usage: xegesis-sign hash <file>")
		}
		h, err := hashFile(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, h)
		return nil
	case "sign":
		fs := flag.NewFlagSet("sign", flag.ContinueOnError)
		version := fs.String("version", "", "the version the installer is released as, e.g. 1.2.3")
		keyFile := fs.String("key", "", "file holding the private key (default: the XEGESIS_UPDATE_KEY variable)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *version == "" || fs.NArg() != 1 {
			return errors.New("usage: xegesis-sign sign -version V [-key file] <file>")
		}
		key := getenv("XEGESIS_UPDATE_KEY")
		if *keyFile != "" {
			b, err := os.ReadFile(*keyFile)
			if err != nil {
				return err
			}
			key = string(b)
		}
		if strings.TrimSpace(key) == "" {
			return errors.New("no private key: set XEGESIS_UPDATE_KEY or pass -key")
		}
		h, err := hashFile(fs.Arg(0))
		if err != nil {
			return err
		}
		sig, err := updatesig.Sign(key, *version, h)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, sig)
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return updatesig.Sum(f)
}
