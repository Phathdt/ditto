package cmd

import (
	"ditto/listener"
	"ditto/shared/common"
	"ditto/shared/component/pgxc"
	"ditto/shared/component/redis"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	sctx "github.com/phathdt/service-context"
	"github.com/spf13/cobra"
)

const (
	serviceName = "ditto"
	version     = "1.0.0"
)

func newServiceCtx() sctx.ServiceContext {
	return sctx.NewServiceContext(
		sctx.WithName(serviceName),
		sctx.WithComponent(pgxc.New(common.KeyCompPgx)),
		sctx.WithComponent(redis.New(common.KeyCompRedis, "")),
	)
}

var rootCmd = &cobra.Command{
	Use:   serviceName,
	Short: fmt.Sprintf("start %s", serviceName),
	Run: func(cmd *cobra.Command, args []string) {
		serviceCtx := newServiceCtx()

		logger := sctx.GlobalLogger().GetLogger("service")

		time.Sleep(time.Second * 1)

		if err := serviceCtx.Load(); err != nil {
			logger.Fatal(err)
		}

		lis := listener.New(serviceCtx)

		go func() {
			if err := lis.Process(); err != nil {
				panic(err)
			}
		}()

		// gracefully shutdown
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		_ = serviceCtx.Stop()
		logger.Info("Server exited")
	},
}

func Execute() {
	rootCmd.AddCommand(outEnvCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
