package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"

	solana_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/spf13/cobra"
	"go.blockdaemon.com/pyth"
	"go.blockdaemon.com/pythian/cmd"
	"go.blockdaemon.com/pythian/jsonrpc"
	"go.blockdaemon.com/pythian/schedule"
	pythian_server "go.blockdaemon.com/pythian/server"
	"go.blockdaemon.com/pythian/signer"
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
	serverFlags.AddFlagSet(cmd.FlagSetSigner)
	serverFlags.StringVar(&serverListenFlag, "listen", ":8090", "Listen address")
}

func runServer(_ *cobra.Command, _ []string) {
	log.Info("Initializing")
	defer log.Info("Shutdown completed")

	// Create root application context.
	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Print message when exit is about to occur.
	group.Go(func() error {
		defer log.Info("Exit requested")
		<-ctx.Done()
		return nil
	})

	// Create RPC/WebSocket client to Pyth on-chain program.
	solanaRpcUrl, err := cmd.GetRPCFlag()
	cobra.CheckErr(err)
	solanaWsUrl, err := cmd.GetWSFlag()
	cobra.CheckErr(err)
	pythEnv, err := cmd.GetPythEnv()
	cobra.CheckErr(err)
	pythClient := pyth.NewClient(pythEnv, solanaRpcUrl.String(), solanaWsUrl.String())
	solanaRPC := solana_rpc.New(solanaRpcUrl.String())

	// Create transaction signer.
	txSigner, err := signer.NewSigner(cmd.GetPrivateKeyPath(), pythEnv.Program)
	cobra.CheckErr(err)
	log.Info("Signer initialized", zap.Stringer("pubkey", txSigner.Pubkey()))

	// Create recent block hash monitor.
	log.Info("Starting block hash monitor")
	blockhashes, err := schedule.NewBlockHashMonitor(ctx, solanaRPC)
	if err != nil {
		log.Fatal("Failed to set up blockhash monitor", zap.Error(err))
	}
	blockhashes.Log = log.Named("blockhash")
	group.Go(func() error {
		defer log.Info("Stopped block hash monitor")
		blockhashes.Run(ctx)
		return nil
	})

	// Create slot monitor.
	log.Info("Starting slot monitor")
	slots := schedule.NewSlotMonitor(solanaWsUrl.String())
	slots.Log = log.Named("slots")
	group.Go(func() error {
		defer log.Info("Stopped slot monitor")
		return slots.Run(ctx)
	})

	// Create update buffer.
	buffer := schedule.NewBuffer()

	// Create scheduler.
	sched := schedule.NewScheduler(buffer, blockhashes, txSigner, solanaRPC)
	sched.Log = log.Named("scheduler")
	log.Info("Starting publish scheduler")
	group.Go(func() error {
		defer log.Info("Stopped publish scheduler")
		sched.Run(ctx, slots.Updates())
		return nil
	})

	// Create Pythian JSON-RPC handler.
	rpc := pythian_server.NewHandler(pythClient, buffer, txSigner.Pubkey(), slots)

	// Start HTTP server.
	log.Info("Starting HTTP server", zap.String("listen", serverListenFlag))
	group.Go(func() error {
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

	log.Info("Pythian running 🔮")

	// Wait for all modules to exit.
	if err := group.Wait(); err != nil {
		log.Error("Crashed", zap.Error(err))
	}
}
