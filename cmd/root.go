package cmd

import (
	"ditto/listener"
	"ditto/shared/common"
	"ditto/shared/component/pgxc"
	"ditto/shared/component/watermillapp/natspub"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	sctx "github.com/viettranx/service-context"
)

const (
	serviceName = "ditto"
	version     = "1.0.0"
)

func newServiceCtx() sctx.ServiceContext {
	return sctx.NewServiceContext(
		sctx.WithName(serviceName),
		sctx.WithComponent(pgxc.New(common.KeyCompPgx)),
		sctx.WithComponent(natspub.NewNatsPub(common.KeyCompNatsPub)),
	)
}

var rootCmd = &cobra.Command{
	Use:   serviceName,
	Short: fmt.Sprintf("start %s", serviceName),
	Run: func(cmd *cobra.Command, args []string) {
		serviceCtx := newServiceCtx()

		logger := sctx.GlobalLogger().GetLogger("service")

		time.Sleep(time.Second * 5)

		if err := serviceCtx.Load(); err != nil {
			logger.Fatal(err)
		}

		lis := listener.New(serviceCtx)

		if err := lis.Process(); err != nil {
			panic(err)
		}
	},
}

func Execute() {
	rootCmd.AddCommand(outEnvCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
