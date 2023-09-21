package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/adrg/xdg"
	"github.com/go-errors/errors"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

var (
	configFilePath     string
	boardsDataFilePath string
	userConf           = koanf.New(".")
	boardsData         = koanf.New(".")
	logger             *zap.SugaredLogger
)

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

	// TODO prompt for missing properties and record in file
	//if errors.Is(err, fs.ErrNotExist) {
	//}
	var err error
	configFilePath, err = xdg.ConfigFile("mlog/config.toml")
	if err != nil {
		log.Fatalf("error loading config: %+v", err)
	}

	boardsDataFilePath, err = xdg.DataFile("mlog/boards.toml")
	if err != nil {
		log.Fatalf("error loading config: %+v", err)
	}

	// TODO version command (in addition to -v)
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
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "debug", Aliases: []string{"d"}},
		},
		Commands: cli.Commands{
			{
				Name:        "configuration",
				Aliases:     []string{"cfg"},
				Description: "Print the path of config files on your system",
				Action: func(cCtx *cli.Context) error {
					fmt.Printf("User configuration path: %s\n", configFilePath)
					fmt.Printf("Board data path:         %s\n", boardsDataFilePath)

					if err := boardsData.Load(file.Provider(boardsDataFilePath), toml.Parser()); err == nil {
						description := boardsData.String("description")
						if description != "" {
							fmt.Println("- Description: " + description)
						}
					}

					return nil
				},
			},
			{
				Name:        "update",
				Aliases:     []string{"u"},
				Description: "Fetch the latest boards.toml configuration",
				Action: func(cCtx *cli.Context) error {
					boardsURL := "https://denis-engcom.github.io/mlog/boards.toml"
					boardsResponse, err := http.Get(boardsURL)
					if err != nil {
						return err
					}
					defer boardsResponse.Body.Close()

					// Download into a temporary file.
					// When everything looks good, replace real file at the end as a final step.
					boardsFile, err := os.Create(boardsDataFilePath + ".tmp")
					if err != nil {
						return err
					}
					defer boardsFile.Close()

					n, err := io.Copy(boardsFile, boardsResponse.Body)
					if err != nil {
						return err
					}
					// Close early to allow the upcoming rename to work.
					boardsFile.Close()

					err = os.Rename(boardsDataFilePath+".tmp", boardsDataFilePath)
					if err != nil {
						return err
					}
					fmt.Printf("GET %s (%d bytes) - successful\n", boardsURL, n)
					fmt.Printf("Saved to %s\n", boardsDataFilePath)

					boardsData = koanf.New(".")
					if err := boardsData.Load(file.Provider(boardsDataFilePath), toml.Parser()); err == nil {
						description := boardsData.String("description")
						if description != "" {
							fmt.Println("- Description: " + description)
						}
					}

					fmt.Println("Update complete without errors")

					return nil
				},
			},
			{
				Name:        "month",
				Aliases:     []string{"m"},
				ArgsUsage:   "<yyyy-mm>",
				Description: "Print boards.toml information for a given month",
				Action: func(cCtx *cli.Context) error {
					err := loadConf()
					if err != nil {
						return err
					}

					return checkMonth(cCtx.Args().First())
				},
			},
			{
				Name:        "create-one",
				Aliases:     []string{"co"},
				ArgsUsage:   "<yyyy-mm-dd> <item-description> <hours>",
				Description: "Create one log entry with info provided on the command line",
				Action: func(cCtx *cli.Context) error {
					err := loadConf()
					if err != nil {
						return err
					}

					mondayAPIClient := NewMondayAPIClient()

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
					err := loadConf()
					if err != nil {
						return err
					}

					mondayAPIClient := NewMondayAPIClient()

					return getBoardByID(mondayAPIClient, cCtx.Args().First())
				},
			},
		},
		// Adapt error handling to...
		// * printing stack traces during debug mode
		// * using errors.As to get ExitCoder at any level for printing
		ExitErrHandler: func(cCtx *cli.Context, err error) {
			var sErr *errors.Error
			if cCtx.Bool("debug") && errors.As(err, &sErr) {
				fmt.Fprint(cli.ErrWriter, sErr.ErrorStack())
			} else if cmErr := CommandMessager(nil); errors.As(err, &cmErr) {
				fmt.Fprintln(cli.ErrWriter, cmErr.Message())
			} else if err != nil {
				fmt.Fprintf(cli.ErrWriter, "%v\n", err)
			}

			if ecErr := cli.ExitCoder(nil); errors.As(err, &ecErr) {
				cli.OsExiter(ecErr.ExitCode())
			} else if err != nil {
				cli.OsExiter(1)
			}
		},
	}

	app.Run(os.Args)
}

func loadConf() error {
	if err := userConf.Load(file.Provider(configFilePath), toml.Parser()); err != nil {
		// log.Fatalf("error loading config: %+v", err)
		// "See config.example.toml on github.com/denis-engcom/mlog for instructions on how to fill a configuration file"
		return WrapWithStack(err, "Unable to parse user configuration file: "+configFilePath)
	}
	if err := boardsData.Load(file.Provider(boardsDataFilePath), toml.Parser()); err != nil {
		// log.Fatalf("error loading config: %+v", err)
		// "Please run `mlog update` to fetch the latest boards configuration file"
		return WrapWithStack(err, "Unable to parse boards configuration file: "+boardsDataFilePath)
	}
	return nil
}

func checkMonth(monthYYYYMM string) error {
	monthKey := fmt.Sprintf("months.%s", monthYYYYMM)
	var monthConf Month
	err := boardsData.Unmarshal(monthKey, &monthConf)
	if err != nil {
		return WrapWithStack(err, "Unable to parse boards configuration file: "+boardsDataFilePath)
	}
	if monthConf.BoardID == 0 {
		msg := fmt.Sprintf("boards.toml: month not found: %q", monthYYYYMM)
		return WithStack(msg)
	}

	monthMap := map[string]interface{}{
		"board_id": monthConf.BoardID,
		"days":     monthConf.Days,
	}
	monthBytes, err := toml.Parser().Marshal(monthMap)
	if err != nil {
		return WrapWithStack(err, "Error: unable to parse boards configuration")
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
