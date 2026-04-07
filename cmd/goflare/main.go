package main

import (
    "flag"
    "fmt"
    "os"

    "github.com/tinywasm/goflare"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, goflare.Usage())
        os.Exit(1)
    }

    env := flag.String("env", ".env", "path to .env file")
    flag.CommandLine.Parse(os.Args[2:])

    var err error
    switch os.Args[1] {
    case "init":
        err = goflare.RunInit(*env, os.Stdin, os.Stdout)
    case "build":
        err = goflare.RunBuild(*env, os.Stdout)
    case "deploy":
        err = goflare.RunDeploy(*env, os.Stdin, os.Stdout)
    default:
        fmt.Fprintln(os.Stderr, goflare.Usage())
        os.Exit(1)
    }

    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
