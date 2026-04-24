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

	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	env := fs.String("env", ".env", "path to .env file")

	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "init":
		err = goflare.RunInit(*env, os.Stdin, os.Stdout)
	case "auth":
		authFs := flag.NewFlagSet("auth", flag.ExitOnError)
		reset := authFs.Bool("reset", false, "delete saved token")
		check := authFs.Bool("check", false, "verify saved token")
		authFs.Parse(os.Args[2:])
		err = goflare.RunAuth(*env, os.Stdin, os.Stdout, *reset, *check)
	case "build":
		err = goflare.RunBuild(*env, os.Stdout)
	case "deploy":
		err = goflare.RunDeploy(*env, os.Stdin, os.Stdout)
	default:
		fmt.Fprint(os.Stderr, goflare.Usage())
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
