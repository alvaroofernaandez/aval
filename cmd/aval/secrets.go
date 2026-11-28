package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/secrets"
)

const secretsUsage = "usage: aval secrets set NAME | get NAME [--show] | list | rm NAME | exec NAME[,NAME2] -- cmd [args]"

// secretsDeps is what the command needs from the machine: the keychain and the
// way a child process starts.
type secretsDeps struct {
	store secrets.Store
	exec  func(path string, argv, env []string) error
}

func realSecretsDeps() secretsDeps { return secretsDeps{store: secrets.Default(), exec: execReplace} }

// runSecrets runs `aval secrets <sub>`. It never writes a value except for
// `get --show`.
func runSecrets(sub []string, c cmdIO, d secretsDeps) error {
	if len(sub) == 0 {
		return newUsage(secretsUsage)
	}
	switch sub[0] {
	case "set":
		return secretsSet(sub[1:], c, d)
	case "get":
		return secretsGet(sub[1:], c, d)
	case "list":
		names, err := d.store.List()
		for _, n := range names {
			fmt.Fprintln(c.stdout, n)
		}
		return err
	case "rm":
		if len(sub) != 2 {
			return newUsage(secretsUsage)
		}
		return d.store.Delete(sub[1])
	case "exec":
		return secretsExec(sub[1:], d)
	}
	return newUsage(secretsUsage)
}

func secretsSet(args []string, c cmdIO, d secretsDeps) error {
	if len(args) != 1 || !secrets.ValidName(args[0]) {
		return newUsage(secretsUsage)
	}
	v, err := bufio.NewReader(io.LimitReader(c.stdin, 1<<20)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return d.store.Set(args[0], strings.TrimRight(v, "\r\n"))
}

func secretsGet(args []string, c cmdIO, d secretsDeps) error {
	show := slices.Contains(args, "--show")
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "--show" })
	if len(args) != 1 {
		return newUsage(secretsUsage)
	}
	v, err := d.store.Get(args[0])
	if err == nil && show {
		fmt.Fprintln(c.stdout, v)
	}
	return err
}

func secretsExec(args []string, d secretsDeps) error {
	i := slices.Index(args, "--")
	if i != 1 || i == len(args)-1 {
		return newUsage(secretsUsage)
	}
	env, err := secrets.ChildEnv(d.store, strings.Split(args[0], ","), os.Environ())
	if err != nil {
		return err
	}
	path, err := exec.LookPath(args[i+1])
	if err != nil {
		return err
	}
	return d.exec(path, args[i+1:], env)
}
