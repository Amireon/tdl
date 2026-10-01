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
	"go.etcd.io/bbolt"

	"github.com/iyear/tdl/app/web"
	"github.com/iyear/tdl/pkg/consts"
	"github.com/iyear/tdl/pkg/kv"
)

func NewWeb() *cobra.Command {
	var (
		port      int
		noBrowser bool
		webDir    string
	)

	cmd := &cobra.Command{
		Use:     "web",
		Short:   "Start a local web UI for tdl",
		GroupID: groupTools.ID,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			ns := viper.GetString(consts.FlagNamespace)
			kvd, err := kv.From(ctx).Open(ns)
			if errors.Is(err, bbolt.ErrTimeout) {
				// a stale `tdl web` instance holds the db lock: kill it and
				// take over, so re-running the command acts as a restart
				dbPath := filepath.Join(
					viper.GetStringMapString(consts.FlagStorage)["path"], ns)
				color.Yellow("Database %s is locked by a stale tdl web process, terminating it...", dbPath)
				if kerr := terminateWebLockHolders(dbPath); kerr != nil {
					color.Yellow("auto-terminate failed: %v", kerr)
				}
				// bbolt waits up to 1s per attempt; retry until the OS
				// releases the killed process's file lock
				for i := 0; i < 3 && err != nil; i++ {
					time.Sleep(300 * time.Millisecond)
					kvd, err = kv.From(ctx).Open(ns)
				}
			}
			if err != nil {
				return errors.Wrap(err, "open kv storage")
			}

			dataDir := filepath.Join(consts.DataDir, "web")
			// migrate data from the pre-rename "gui" directory
			if oldDir := filepath.Join(consts.DataDir, "gui"); !fileExists(dataDir) && fileExists(oldDir) {
				if err := os.Rename(oldDir, dataDir); err != nil {
					return errors.Wrap(err, "migrate gui data dir")
				}
			}

			store, err := web.NewStore(dataDir)
			if err != nil {
				return errors.Wrap(err, "open web store")
			}

			settings := store.Settings()
			viper.Set(consts.FlagThreads, settings.Threads)
			viper.Set(consts.FlagLimit, settings.Limit)

			engine := web.NewEngine(ctx, store, kvd, viper.GetString(consts.FlagProxy))
			engine.Start()

			srv := web.NewServer(engine, webDir)
			url, err := srv.ListenAndServe(port)
			if err != nil {
				return err
			}

			color.Green("tdl web UI is running at %s", url)
			if !noBrowser {
				if err := web.OpenBrowser(url); err != nil {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
