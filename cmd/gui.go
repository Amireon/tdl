package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/iyear/tdl/app/gui"
	"github.com/iyear/tdl/pkg/consts"
	"github.com/iyear/tdl/pkg/kv"
)

func NewGUI() *cobra.Command {
	var (
		port      int
		noBrowser bool
		webDir    string
	)

	cmd := &cobra.Command{
		Use:     "gui",
		Short:   "Start a local web GUI for tdl",
		GroupID: groupTools.ID,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			kvd, err := kv.From(ctx).Open(viper.GetString(consts.FlagNamespace))
			if err != nil {
				return errors.Wrap(err, "open kv storage")
			}

			store, err := gui.NewStore(filepath.Join(consts.DataDir, "gui"))
			if err != nil {
				return errors.Wrap(err, "open gui store")
			}

			settings := store.Settings()
			viper.Set(consts.FlagThreads, settings.Threads)
			viper.Set(consts.FlagLimit, settings.Limit)

			engine := gui.NewEngine(ctx, store, kvd, viper.GetString(consts.FlagProxy))
			engine.Start()

			srv := gui.NewServer(engine, webDir)
			url, err := srv.ListenAndServe(port)
			if err != nil {
				return err
			}

			color.Green("tdl GUI is running at %s", url)
			if !noBrowser {
				if err := gui.OpenBrowser(url); err != nil {
					color.Yellow("failed to open browser: %v", err)
				}
			}

			<-ctx.Done()
			fmt.Println("\nShutting down...")

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			// wait for the telegram client, executor and in-flight tasks
			// to stop before kv storage is closed by PersistentPostRun
			if err := engine.Shutdown(shutdownCtx); err != nil {
				color.Yellow("engine shutdown: %v", err)
			}
			_ = srv.Shutdown(shutdownCtx)
			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 16686, "web server port (auto-incremented on conflict)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "do not open browser automatically")
	cmd.Flags().StringVar(&webDir, "web-dir", "", "serve frontend from this directory instead of embedded assets (dev)")

	return cmd
}
