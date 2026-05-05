// hashpw is a tiny CLI: read a password from argv, print a bcrypt hash that
// matches `pkg/crypto.HashPassword` (cost 12). Use this when seeding the
// initial admin row via SQL.
//
//	go run ./scripts/hashpw 'YourStrongPassword!'
package main

import (
	"fmt"
	"os"

	appcrypto "github.com/menta2k/iris/backend/pkg/crypto"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hashpw '<password>'")
		os.Exit(2)
	}
	hash, err := appcrypto.HashPassword(os.Args[1], appcrypto.MinBcryptCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hashpw:", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
