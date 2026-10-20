// Command admin is the break-glass CLI (ADR-059). It talks to the database
// directly, so it works when nobody can sign in.
//
//	admin ensure-maintainer -email ops@example.com -name "Ops" < password.txt
//
// creates the account, or restores an existing one: it becomes an enabled
// maintainer with the given local password (read from stdin, first line).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/config"
	"archmap/api/internal/platform/db"
	"archmap/api/internal/platform/password"
	"archmap/api/internal/user"
	userrepo "archmap/api/internal/user/repository"
)

func main() {
	if err := run(os.Args[1:], os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader) error {
	if len(args) == 0 || args[0] != "ensure-maintainer" {
		return errors.New("usage: admin ensure-maintainer -email <email> -name <name>  (password on stdin)")
	}
	fs := flag.NewFlagSet("ensure-maintainer", flag.ContinueOnError)
	email := fs.String("email", "", "sign-in email")
	name := fs.String("name", "", "display name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *email == "" || *name == "" {
		return errors.New("-email and -name are required")
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	pw := strings.TrimRight(line, "\r\n")
	if err := password.Validate(pw); err != nil {
		return err
	}

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	id, err := user.NewService(userrepo.New(pool)).EnsureLocal(ctx, user.LocalAccount{
		Email: *email, Name: *name, Role: authctx.RoleMaintainer, Password: pw,
	})
	if err != nil {
		return err
	}
	fmt.Printf("maintainer %s (%s) is enabled with a local password\n", *email, id)
	return nil
}
