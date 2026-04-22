// pindoc-admin is a tiny CLI for operator-editable server settings. Lives
// between "raw psql" and "Settings UI" — the UI lands in V1.5 with auth;
// until then this is how a self-host operator changes public_base_url or
// other runtime settings without a server restart.
//
// Usage:
//
//	pindoc-admin list
//	pindoc-admin get public_base_url
//	pindoc-admin set public_base_url https://wiki.acme.dev
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: pindoc-admin <command> [args]")
		fmt.Fprintln(os.Stderr, "commands:")
		fmt.Fprintln(os.Stderr, "  list                 — list editable keys")
		fmt.Fprintln(os.Stderr, "  get <key>            — print current value")
		fmt.Fprintln(os.Stderr, "  set <key> <value>    — update a setting (hot, no restart)")
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load", "err", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open", "err", err, "hint", "is docker compose up -d db running?")
		os.Exit(1)
	}
	defer pool.Close()

	store, err := settings.New(ctx, pool)
	if err != nil {
		// Most likely cause: migration 0007 hasn't run yet (server never
		// started). Point the operator there instead of at psql.
		logger.Error("settings load", "err", err, "hint", "start pindoc-server once to apply migrations 0001-0007, then retry")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		v := store.Get()
		fmt.Printf("public_base_url = %q\n", v.PublicBaseURL)
		fmt.Printf("updated_at      = %s\n", v.UpdatedAt.Format(time.RFC3339))
		fmt.Println()
		fmt.Println("Keys you can set:")
		for _, k := range settings.AllKeys() {
			fmt.Printf("  %s\n", k)
		}

	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "get: missing key")
			os.Exit(2)
		}
		v := store.Get()
		switch args[1] {
		case "public_base_url":
			fmt.Println(v.PublicBaseURL)
		default:
			fmt.Fprintf(os.Stderr, "unknown key: %s\n", args[1])
			os.Exit(2)
		}

	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "set: missing <key> <value>")
			os.Exit(2)
		}
		key := args[1]
		// Allow spaces in values ("set x hello world") by joining
		// everything after the key.
		value := strings.Join(args[2:], " ")
		if err := store.Set(ctx, key, value); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Printf("updated %s = %q\n", key, value)
		fmt.Fprintln(os.Stderr, "note: pindoc-api and pindoc-server cache settings at startup. Restart them to pick up this change. (Hot-reload lands in V1.x.)")

	default:
		flag.Usage()
		os.Exit(2)
	}
}
