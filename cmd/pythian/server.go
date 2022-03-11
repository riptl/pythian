package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.blockdaemon.com/pyth"
	"go.blockdaemon.com/pythian/cmd"
	"go.blockdaemon.com/pythian/pkg/jsonrpc"
	"go.blockdaemon.com/pythian/pkg/server"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var serverCmd = cobra.Command{
	Use:   "server",
	Short: "Run pythian server",
	Args:  cobra.NoArgs,
	Run:   runServer,
}

var (
	serverFlags      = serverCmd.Flags()
	serverListenFlag string
)

func init() {
	rootCmd.AddCommand(&serverCmd)
	serverFlags.AddFlagSet(cmd.FlagSetRPC)
	serverFlags.StringVar(&serverListenFlag, "listen", ":8090", "Listen address")
}

func runServer(_ *cobra.Command, _ []string) {
	log.Info("Starting pythian server")
	defer log.Info("Exiting now")

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Create Pyth client.
	var pythEnv pyth.Env
	switch *cmd.FlagNetwork {
	case "devnet":
		pythEnv = pyth.Devnet
	case "testnet":
		pythEnv = pyth.Testnet
	case "mainnet":
		pythEnv = pyth.Mainnet
	default:
		cobra.CheckErr("unsupported network: " + *cmd.FlagNetwork)
	}

	solanaRpcUrl, err := cmd.GetRPCFlag()
	cobra.CheckErr(err)
	solanaWsUrl, err := cmd.GetWSFlag()
	cobra.CheckErr(err)
	pythClient := pyth.NewClient(pythEnv, solanaRpcUrl.String(), solanaWsUrl.String())

	rpc := server.NewHandler(pythClient)

	group.Go(func() error {
		log.Info("Starting HTTP server", zap.String("listen", serverListenFlag))
		defer log.Info("Stopped HTTP server")

		rpcServer := jsonrpc.NewServer(rpc)
		http.Handle("/", rpcServer)

		server := http.Server{Addr: serverListenFlag}
		go func() {
			<-ctx.Done()
			_ = server.Close()
		}()

		if err := server.ListenAndServe(); errors.Is(err, http.ErrServerClosed) {
			return nil
		} else {
			return err
		}
	})

	if err := group.Wait(); err != nil {
		log.Error("Crashed", zap.Error(err))
	} else {
		log.Info("Shutting down")
	}
}
