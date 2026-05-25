package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tinywasm/goflare"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, goflare.Usage())
		os.Exit(1)
	}

	var env string
	var reset, check bool

	fs := flag.NewFlagSet("goflare", flag.ExitOnError)
	fs.StringVar(&env, "env", ".env", "path to .env file")

	cmd := ""
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "init":
		fs.Parse(os.Args[2:])
		err = goflare.RunInit(env, os.Stdin, os.Stdout)
	case "auth":
		fs.BoolVar(&reset, "reset", false, "delete saved token")
		fs.BoolVar(&check, "check", false, "verify saved token")
		fs.Parse(os.Args[2:])
		err = goflare.RunAuth(env, os.Stdin, os.Stdout, reset, check)
	case "build":
		fs.Parse(os.Args[2:])
		err = goflare.RunBuild(env, os.Stdout)
	case "deploy":
		fs.Parse(os.Args[2:])
		err = goflare.RunDeploy(env, os.Stdin, os.Stdout)
	case "d1":
		sub := ""
		if len(os.Args) >= 3 {
			sub = os.Args[2]
		}
		var dbName string
		fs.StringVar(&dbName, "db-name", "", "D1 database name (default: PROJECT_NAME)")
		fs.Parse(os.Args[3:])
		switch sub {
		case "init":
			err = goflare.RunD1InitCmd(env, dbName)
		default:
			fmt.Fprintf(os.Stderr, "unknown d1 subcommand: %s\n", sub)
			os.Exit(1)
		}
	default:
		fmt.Fprint(os.Stderr, goflare.Usage())
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
