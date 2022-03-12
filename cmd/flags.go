package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.blockdaemon.com/pyth"
)

var (
	FlagSetCommon = pflag.NewFlagSet("common", pflag.ExitOnError)
	FlagNetwork   = FlagSetCommon.String("network", "mainnet", "Solana network (devnet, testnet, mainnet)")

	FlagSetRPC = pflag.NewFlagSet("rpc", pflag.ExitOnError)
	flagRPC    = pflag.String("rpc", "https://api.mainnet-beta.solana.com", "RPC URL")
	flagWS     = pflag.String("ws", "", "WebSocket RPC URL")

	FlagSetSigner  = pflag.NewFlagSet("signer", pflag.ExitOnError)
	flagPrivateKey = pflag.String("private-key-file", "", "Path to private key file")
)

func GetRPCFlag() (*url.URL, error) {
	if *flagRPC == "" {
		return nil, fmt.Errorf("missing RPC flag")
	}
	u, err := url.Parse(*flagRPC)
	if err != nil {
		return nil, fmt.Errorf("invalid RPC URL: %s", err)
	}
	return u, nil
}

func GetWSFlag() (*url.URL, error) {
	if *flagWS == "" {
		u, err := GetRPCFlag()
		if err != nil {
			goto missingFlag
		}
		switch u.Scheme {
		case "http":
			u.Scheme = "ws"
		case "https":
			u.Scheme = "wss"
		default:
			goto missingFlag
		}
		return u, nil

	missingFlag:
		return nil, fmt.Errorf("missing WebSocket RPC flag")
	}

	u, err := url.Parse(*flagWS)
	if err != nil {
		return nil, fmt.Errorf("invalid WebSocket URL: %s", err)
	}
	return u, nil
}

func GetPythEnv() (pyth.Env, error) {
	var pythEnv pyth.Env
	switch *FlagNetwork {
	case "devnet":
		pythEnv = pyth.Devnet
	case "testnet":
		pythEnv = pyth.Testnet
	case "mainnet":
		pythEnv = pyth.Mainnet
	default:
		return pyth.Env{}, fmt.Errorf("unsupported network: %s", *FlagNetwork)
	}
	return pythEnv, nil
}

func GetPrivateKeyPath() string {
	v := *flagPrivateKey
	if v == "" {
		cobra.CheckErr("Missing private key flag")
	}
	return v
}
