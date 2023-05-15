package main

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"log"
	"os"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

var logger *zap.SugaredLogger

func main() {
	devLoggerConfig := zap.NewDevelopmentConfig()
	devLoggerConfig.Level.SetLevel(zap.ErrorLevel)
	devLogger := zap.Must(devLoggerConfig.Build())
	defer devLogger.Sync()
	logger = devLogger.Sugar()

	k := koanf.New(".")
	if err := k.Load(file.Provider("config.toml"), toml.Parser()); err != nil {
		log.Fatalf("error loading config: %+v", err)
	}

	mondayAPIClient := NewMondayAPIClient(
		k.MustString("api_access_token"),
		k.MustString("logging_user_id"),
		k.MustString("person_column_id"),
		k.MustString("hours_column_id"))

	app := &cli.App{
		Name:        "mlog",
		Usage:       "Processes timelog input and generates aggregated event information.",
		Description: `Monday logging CLI is a tool to help create pulses in a particular fashion on Monday`,
		// Default output is
		// mlog [global options] command [command options] [arguments...]
		UsageText:            `mlog command [arguments...]`,
		Version:              "0.1.0",
		HideHelpCommand:      true,
		EnableBashCompletion: true,
		Commands: cli.Commands{
			{
				Name:    "get-board",
				Aliases: []string{"gb"},
				Usage:   "Get board information to inform the user on creating logs against this board",
				Action: func(cCtx *cli.Context) error {
					return getBoard(mondayAPIClient, cCtx.Args().First())
				},
			},
			{
				Name:    "create-one",
				Aliases: []string{"co"},
				Usage:   "Create one log entry with info provided on the command line",
				Action: func(cCtx *cli.Context) error {
					args := cCtx.Args()
					return createOne(mondayAPIClient, args.Get(0), args.Get(1), args.Get(2), args.Get(3))
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatalf("%+v", err)
	}
}

func getBoard(mondayAPIClient *MondayAPIClient, boardID string) error {
	logger.Infow("getBoard", "boardID", boardID)
	board, err := mondayAPIClient.GetBoard(boardID)
	if err != nil {
		return err
	}
	boardOutput, err := json.Marshal(board)
	fmt.Println(string(boardOutput))
	return nil
}

func createOne(mondayAPIClient *MondayAPIClient, boardID, groupID, itemName, hours string) error {
	logger.Infow("createOne", "boardID", boardID, "groupID", groupID, "itemName", itemName, "hours", hours)
	res, err := mondayAPIClient.CreateLogItem(boardID, groupID, itemName, hours)
	if err != nil {
		return err
	}
	fmt.Printf("https://magicboard.monday.com/boards/%s/pulses/%s\n", boardID, res.Create_Item.ID)
	return nil
}
