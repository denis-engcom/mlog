package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/adrg/xdg"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

var (
	userConf   = koanf.New(".")
	boardsData = koanf.New(".")
	logger     *zap.SugaredLogger
)

// type BoardsData struct {
// 	Months map[string]Month `koanf:"months"`
// }

// go embed boards.toml
// var boardsTOML []byte

type Month struct {
	BoardID uint64            `koanf:"board_id"`
	Days    map[string]string `koanf:"days"`
}

func main() {
	devLoggerConfig := zap.NewDevelopmentConfig()
	devLoggerConfig.Level.SetLevel(zap.ErrorLevel)
	devLogger := zap.Must(devLoggerConfig.Build())
	defer devLogger.Sync()
	logger = devLogger.Sugar()

	// TODO relocate conf parsing to happen in subcommand

	// TODO prompt for missing properties and record in file
	//if errors.Is(err, fs.ErrNotExist) {
	//}
	configFilePath, err := xdg.ConfigFile("mlog/config.toml")
	if err != nil {
		log.Fatalf("error loading config: %+v", err)
	}

	if err := userConf.Load(file.Provider(configFilePath), toml.Parser()); err != nil {
		log.Fatalf("error loading config: %+v", err)
	}

	// TODO Update `mlog get-boards` to somehow fetch this conf?
	// How best to propagate this conf to cli users?
	// Maybe have ability to download from github? Need convenient CLI login functionality though.
	boardsDataFilePath, err := xdg.DataFile("mlog/boards.toml")
	if err != nil {
		log.Fatalf("error loading config: %+v", err)
	}

	if err := boardsData.Load(file.Provider(boardsDataFilePath), toml.Parser()); err != nil {
		log.Fatalf("error loading config: %+v", err)
	}

	mondayAPIClient := NewMondayAPIClient()

	app := &cli.App{
		Name:        "mlog",
		Usage:       "Processes timelog input and generates aggregated event information.",
		Description: `Monday logging CLI is a tool to help create pulses in a particular fashion on Monday`,
		// Default output is
		// mlog [global options] command [command options] [arguments...]
		UsageText:            `mlog command [arguments...]`,
		Version:              "0.2.0",
		HideHelpCommand:      true,
		EnableBashCompletion: true,

		Commands: cli.Commands{
			{
				Name:        "month",
				Aliases:     []string{"m"},
				ArgsUsage:   "<yyyy-mm>",
				Description: "Print boards.toml information for a given month",
				Action: func(cCtx *cli.Context) error {
					return checkMonth(cCtx.Args().First())
				},
			},
			{
				Name:        "create-one",
				Aliases:     []string{"co"},
				ArgsUsage:   "<yyyy-mm-dd> <item-description> <hours>",
				Description: "Create one log entry with info provided on the command line",
				Action: func(cCtx *cli.Context) error {
					args := cCtx.Args()
					return createOne(mondayAPIClient, args.Get(0), args.Get(1), args.Get(2))
				},
			},
			{
				Name:        "get-board",
				Aliases:     []string{"gb"},
				ArgsUsage:   "<board-id>",
				Description: "(Admin command) get board information by board-id to populate boards.toml",
				Action: func(cCtx *cli.Context) error {
					return getBoardByID(mondayAPIClient, cCtx.Args().First())
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatalf("%+v", err)
	}
}

func checkMonth(monthYYYYMM string) error {
	monthKey := fmt.Sprintf("months.%s", monthYYYYMM)
	var monthConf Month
	err := boardsData.Unmarshal(monthKey, &monthConf)
	if err != nil {
		return err
	}
	if monthConf.BoardID == 0 {
		return errors.Errorf("boards.toml: month not found: %q", monthYYYYMM)
	}

	monthMap := map[string]interface{}{
		"board_id": monthConf.BoardID,
		"days":     monthConf.Days,
	}
	monthBytes, err := toml.Parser().Marshal(monthMap)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", monthBytes)
	return nil
}

func getBoardByID(mondayAPIClient *MondayAPIClient, boardID string) error {
	logger.Debugw("getBoardByID", "boardID", boardID)
	boardIDInt, err := strconv.Atoi(boardID)
	if err != nil {
		return err
	}
	board, err := mondayAPIClient.GetBoardByID(boardIDInt)
	if err != nil {
		return err
	}
	boardOutput, err := json.Marshal(board)
	fmt.Println(string(boardOutput))
	return nil
}

func getBoardIDForMonth(month string) int {
	key := fmt.Sprintf("months.%s.board_id", month)
	return boardsData.Int(key)
}

func getBoard(mondayAPIClient *MondayAPIClient, monthYYYYMM string) error {
	boardID := getBoardIDForMonth(monthYYYYMM)
	if boardID == 0 {
		return errors.Errorf("boards.toml: board_id not found for month %q", monthYYYYMM)
	}
	logger.Debugw("getBoardByID", "month", monthYYYYMM, "boardID", boardID)
	board, err := mondayAPIClient.GetBoardByID(boardID)
	if err != nil {
		return err
	}
	boardOutput, err := json.Marshal(board)
	fmt.Println(string(boardOutput))
	return nil
}

func getGroupIDForDay(month, day string) string {
	key := fmt.Sprintf("months.%s.days.%s", month, day)
	return boardsData.String(key)
}

func createOne(mondayAPIClient *MondayAPIClient, dayYYYYMMDD, itemName, hours string) error {
	if len(dayYYYYMMDD) != 10 {
		return errors.Errorf("provided day is not in format yyyy-mm-dd: %q", dayYYYYMMDD)
	}
	month := dayYYYYMMDD[0:7]
	boardID := getBoardIDForMonth(month)
	if boardID == 0 {
		return errors.Errorf("boards.toml: board_id not found for month %q", month)
	}
	day := dayYYYYMMDD[7:10]
	groupID := getGroupIDForDay(month, day)
	if groupID == "" {
		return errors.Errorf("boards.toml: group_id not found for month %q and day %q", month, day)
	}
	logger.Debugw("createOne", "day", dayYYYYMMDD, "boardID", boardID, "groupID", groupID, "itemName", itemName, "hours", hours)

	res, err := mondayAPIClient.CreateLogItem(boardID, groupID, itemName, hours)
	if err != nil {
		return err
	}
	fmt.Printf("https://magicboard.monday.com/boards/%d/pulses/%s\n", boardID, res.Create_Item.ID)
	return nil
}
