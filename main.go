package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"

	surveyterm "github.com/AlecAivazis/survey/v2/terminal"
	"github.com/fatih/color"
	"github.com/go-faster/errors"
	"github.com/spf13/viper"
	"go.etcd.io/bbolt"

	"github.com/iyear/tdl/cmd"
	"github.com/iyear/tdl/pkg/consts"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	humanizeErrors := map[error]string{
		surveyterm.InterruptErr: "Interrupted",
	}

	if err := cmd.New().ExecuteContext(ctx); err != nil {
		if errors.Is(err, bbolt.ErrTimeout) {
			// bbolt holds an exclusive flock for the process lifetime, so a
			// timeout always means another live tdl process is running
			db := filepath.Join(
				viper.GetStringMapString(consts.FlagStorage)["path"],
				viper.GetString(consts.FlagNamespace),
			)
			color.Red("Database is locked by another running tdl process:\n\n  %s\n\n"+
				"Find it with:\n\n  lsof %s\n\n"+
				"Then terminate it with `kill <PID>` and try again.", db, db)
			os.Exit(1)
		}

		for e, m := range humanizeErrors {
			if errors.Is(err, e) {
				color.Red("%s", m)
				os.Exit(1)
			}
		}

		color.Red("Error: %+v", err)
		os.Exit(1)
	}
}
