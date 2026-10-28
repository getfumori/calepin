// Command fumori bootstraps, checks, and serves a Vault.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/getfumori/fumori/internal/api"
	"github.com/getfumori/fumori/internal/index"
	"github.com/getfumori/fumori/internal/vault"
	"github.com/getfumori/fumori/internal/webassets"
)

const usage = `Usage:
  fumori bootstrap --vault <path> [--name <name>] [--timezone <IANA zone>]
  fumori check     --vault <path>
  fumori serve     --vault <path> [--state <dir>] [--host 127.0.0.1] [--port 3000]
  fumori reindex   --vault <path> [--state <dir>]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	commands := map[string]func([]string) error{
		"bootstrap": bootstrap, "check": check, "serve": serve, "reindex": reindex,
	}
	run, ok := commands[os.Args[1]]
	if !ok {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "fumori:", err)
		os.Exit(1)
	}
}

func vaultFlag(fs *flag.FlagSet) *string {
	return fs.String("vault", "", "path to the Vault Git repository")
}

func absVault(p string) (string, error) {
	if p == "" {
		return "", errors.New("--vault is required")
	}
	return filepath.Abs(p)
}

func systemZone() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if i := len("zoneinfo/"); len(target) > i {
			for j := 0; j+i <= len(target); j++ {
				if target[j:j+i] == "zoneinfo/" {
					return target[j+i:]
				}
			}
		}
	}
	return "UTC"
}

func bootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	path := vaultFlag(fs)
	name := fs.String("name", "Fumori", "Vault display name")
	zone := fs.String("timezone", systemZone(), "owner IANA time zone")
	fs.Parse(args)
	root, err := absVault(*path)
	if err != nil {
		return err
	}
	id, err := vault.Bootstrap(context.Background(), root, *name, *zone, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("Bootstrapped Vault %s at %s (time zone %s)\n", id, root, *zone)
	return nil
}

func check(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	path := vaultFlag(fs)
	fs.Parse(args)
	root, err := absVault(*path)
	if err != nil {
		return err
	}
	diags, err := vault.Check(root)
	if err != nil {
		return err
	}
	for _, d := range diags {
		for _, p := range d.Problems {
			fmt.Printf("%s: %s\n", d.Path, p)
		}
	}
	if len(diags) > 0 {
		return fmt.Errorf("%d files quarantined", len(diags))
	}
	fmt.Println("Vault is valid.")
	return nil
}

func reindex(args []string) error {
	fs := flag.NewFlagSet("reindex", flag.ExitOnError)
	path := vaultFlag(fs)
	state := fs.String("state", "", "runtime state directory")
	fs.Parse(args)
	root, err := absVault(*path)
	if err != nil {
		return err
	}
	v, err := vault.Open(context.Background(), vault.Options{Root: root, StateDir: *state})
	if err != nil {
		return err
	}
	stateDir := v.StateDir
	v.Close()
	if err := os.RemoveAll(filepath.Join(stateDir, "index")); err != nil {
		return err
	}
	v, err = vault.Open(context.Background(), vault.Options{Root: root, StateDir: stateDir, OpenIndex: index.Open})
	if err != nil {
		return err
	}
	fmt.Println("Search index rebuilt.")
	return v.Close()
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	path := vaultFlag(fs)
	state := fs.String("state", "", "runtime state directory (default $XDG_STATE_HOME/fumori/<vault-id>)")
	host := fs.String("host", "127.0.0.1", "listen host")
	port := fs.Int("port", 3000, "listen port")
	interval := fs.Duration("checkpoint-interval", 30*time.Minute, "Git checkpoint interval")
	fs.Parse(args)
	root, err := absVault(*path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	v, err := vault.Open(ctx, vault.Options{Root: root, StateDir: *state, OpenIndex: index.Open})
	if err != nil {
		return err
	}
	defer v.Close()
	for _, d := range v.Diagnostics() {
		fmt.Fprintf(os.Stderr, "quarantined %s: %v\n", d.Path, d.Problems)
	}
	web := webassets.FS()
	if web == nil {
		fmt.Fprintln(os.Stderr, "warning: this binary has no Web client; serving the API only")
	}
	handler := api.Handler(v, api.Config{
		AutosaveDebounce: 1500 * time.Millisecond, AutosaveMaxDirty: 10 * time.Second,
		CheckpointInterval: *interval, Web: web,
	})
	listener, err := net.Listen("tcp", net.JoinHostPort(*host, fmt.Sprint(*port)))
	if err != nil {
		return err
	}
	if ip := net.ParseIP(*host); *host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		fmt.Fprintf(os.Stderr, "warning: listening on %s without authentication or TLS; use only a trusted network or an authenticating gateway\n", *host)
	}
	go v.RunCheckpoints(ctx, *interval, func(err error) { fmt.Fprintln(os.Stderr, "checkpoint failed:", err) })
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	fmt.Printf("Fumori is serving %s at http://%s\n", v.Manifest.Name, listener.Addr())
	if err := srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if _, err := v.Checkpoint(context.Background()); err != nil {
		return err
	}
	return nil
}
