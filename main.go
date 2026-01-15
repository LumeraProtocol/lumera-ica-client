// Package main boots the Lumera ICA reference client CLI.
package main

import (
	"fmt"
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"

	commands "lumera-ica-client/cmd"
)

func main() {
	// Force Lumera bech32 prefixes for address formatting used in this client.
	sdk.GetConfig().SetBech32PrefixForAccount("lumera", "lumerapub")
	if err := commands.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
