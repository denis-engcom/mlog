package main

import (
	_ "embed"
	"fmt"
	"io"
	// "log"
	"net/http"
	"os"
	"strconv"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/adrg/xdg"
	"github.com/go-errors/errors"
	ptoml "github.com/pelletier/go-toml/v2"
)

var (
	userConfFilePath   string
	boardsConfFilePath string
	logger             *zap.SugaredLogger
)

type UserConf struct {
	APIAccessToken string `toml:"api_access_token"`
	LoggingUserID  string `toml:"logging_user_id"`
}

type BoardsConf struct {
	PersonColumnID string            `toml:"person_column_id"`
	HoursColumnID  string            `toml:"hours_column_id"`
	Description    string            `toml:"description"`
	Months         map[string]*Month `toml:"months"`
}

type Month struct {
	BoardID string            `toml:"board_id"`
	Days    map[string]string `toml:"days"`
}

func main() {
	devLoggerConfig := zap.NewDevelopmentConfig()
	devLoggerConfig.Level.SetLevel(zap.ErrorLevel)
	devLogger := zap.Must(devLoggerConfig.Build())
	defer devLogger.Sync()
	logger = devLogger.Sugar()

	// TODO version command (in addition to -v)
	app := &cli.App{
		Name:        "mlog",
		Usage:       "facilitates log pulse creation on Monday",
		Description: `mlog (Monday logging CLI) is a tool to help create log pulses on Monday.`,
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
			// TODO New route to query items
			// mlog get-items 2023-09
			// mlog get-items 2023-09-01
			// Display using https://github.com/cheynewallace/tabby
			// TODO New route to query item link
			// mlog get-link 5244659133
			// "https://magicboard.monday.com" + relative_link
			{
				Name:        "setup",
				Description: "Setup configuration files needed by the other mlog commands",
				Action:      setup,
			},
			{
				Name:        "update",
				Aliases:     []string{"u"},
				Description: "Fetch the latest boards.toml configuration",
				Action:      update,
			},
			{
				Name:        "create-one",
				Aliases:     []string{"co"},
				ArgsUsage:   "<yyyy-mm-dd> <item-description> <hours>",
				Description: "Create one log entry with info provided on the command line",
				Action: func(cCtx *cli.Context) error {
					userConf, boardsConf, err := loadConf()
					if err != nil {
						return err
					}

					mondayAPIClient := NewMondayAPIClient(
						userConf.APIAccessToken,
						userConf.LoggingUserID,
						boardsConf.PersonColumnID,
						boardsConf.HoursColumnID)

					args := cCtx.Args()
					dayYYYYMMDD, itemName, hours := args.Get(0), args.Get(1), args.Get(2)

					if len(dayYYYYMMDD) != 10 {
						return WithStackF("%q: provided day is not in format yyyy-mm-dd. Exiting.", dayYYYYMMDD)
					}

					monthYYYYMM := dayYYYYMMDD[0:7]
					if len(boardsConf.Months) == 0 {
						return WithStackF("\"months.%s.board_id\": not found in boards configuration. Exiting.", monthYYYYMM)
					}
					month := boardsConf.Months[monthYYYYMM]
					if month == nil || month.BoardID == "" {
						return WithStackF("\"months.%s.board_id\": not found in boards configuration. Exiting.", monthYYYYMM)
					}
					boardIDInt, err := strconv.Atoi(month.BoardID)
					if err != nil {
						return WithStackF("\"months.%s.board_id\": not a number. Exiting.", monthYYYYMM)
					}

					dayDD := dayYYYYMMDD[7:10]
					if len(month.Days) == 0 {
						return WithStackF("\"month.%s.days.%s\": not found in boards configuration. Exiting.", monthYYYYMM, dayDD)
					}
					dayGroupID := month.Days[dayDD]
					if dayGroupID == "" {
						return WithStackF("\"month.%s.days.%s\": not found in boards configuration. Exiting.", monthYYYYMM, dayDD)
					}
					logger.Debugw("createOne", "day", dayYYYYMMDD, "boardID", boardIDInt, "groupID", dayGroupID, "itemName", itemName, "hours", hours)

					res, err := mondayAPIClient.CreateLogItem(boardIDInt, dayGroupID, itemName, hours)
					if err != nil {
						return err
					}
					fmt.Printf("https://magicboard.monday.com/boards/%d/pulses/%s\n", boardIDInt, res.Create_Item.ID)
					return nil
				},
			},
			{
				Name:        "get-board-by-id",
				Aliases:     []string{"gbid"},
				ArgsUsage:   "<board-id>",
				Description: "(Admin command) get board information by board-id to populate boards.toml",
				Action: func(cCtx *cli.Context) error {
					userConf, boardsConf, err := loadConf()
					if err != nil {
						return err
					}

					mondayAPIClient := NewMondayAPIClient(
						userConf.APIAccessToken,
						userConf.LoggingUserID,
						boardsConf.PersonColumnID,
						boardsConf.HoursColumnID)

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

var unableToParseUserConfMsg = "Unable to parse user configuration file.\nRun `mlog setup` for error details."
var unableToParseBoardsConfMsg = "Unable to parse boards configuration file.\nRun `mlog setup` for error details."

func loadConf() (*UserConf, *BoardsConf, error) {
	err := loadConfPaths()
	if err != nil {
		return nil, nil, err
	}

	var userConf UserConf
	err = loadTOML(userConfFilePath, &userConf)
	if err != nil {
		return nil, nil, WrapWithStack(err, unableToParseUserConfMsg)
	}
	if userConf.APIAccessToken == "" || userConf.LoggingUserID == "" {
		return nil, nil, WrapWithStack(err, unableToParseUserConfMsg)
	}

	var boardsConf BoardsConf
	err = loadTOML(boardsConfFilePath, &boardsConf)
	if err != nil {
		return nil, nil, WrapWithStack(err, unableToParseBoardsConfMsg)
	}
	if boardsConf.PersonColumnID == "" || boardsConf.HoursColumnID == "" {
		return nil, nil, WrapWithStack(err, unableToParseBoardsConfMsg)
	}

	return &userConf, &boardsConf, nil
}

func loadConfPaths() error {
	var err error
	userConfFilePath, err = xdg.ConfigFile("mlog/config.toml")
	if err != nil {
		return WrapWithStack(err, "Error: unable to locate user configuration file. Please send a bug report to the developer. Exiting.")
	}

	boardsConfFilePath, err = xdg.DataFile("mlog/boards.toml")
	if err != nil {
		return WrapWithStack(err, "Error: unable to locate boards configuration file. Please send a bug report to the developer. Exiting.")
	}
	return nil
}

func loadTOML(path string, obj any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return ptoml.NewDecoder(file).Decode(obj)
}

func setup(cCtx *cli.Context) error {
	err := loadConfPaths()
	if err != nil {
		return err
	}

	validConfiguration := true
	fmt.Printf("User configuration path:   %s\n", userConfFilePath)
	var userConf UserConf
	err = loadTOML(userConfFilePath, &userConf)
	if err != nil {
		fmt.Println("❌ Unable to parse file (missing or incorrectly formatted)")
		fmt.Println("❌ Missing api_access_token")
		fmt.Println("❌ Missing logging_user_id")
		validConfiguration = false
	} else {
		apiAccessToken := userConf.APIAccessToken
		loggingUserID := userConf.LoggingUserID
		if apiAccessToken != "" && loggingUserID != "" {
			fmt.Println("✅ File is valid")
		} else {
			if apiAccessToken == "" {
				fmt.Println("❌ Missing api_access_token")
				validConfiguration = false
			}
			if loggingUserID == "" {
				fmt.Println("❌ Missing logging_user_id")
				validConfiguration = false
			}
		}
	}

	if !validConfiguration {
		fmt.Println("(skipping boards configuration)")
		return WrapWithStack(err, "The user configuration has one or more validation errors.\nRefer to github.com/denis-engcom/mlog - config.example.toml for how to configure the file properly.")
	}

	fmt.Printf("Boards configuration path: %s\n", boardsConfFilePath)
	var boardsConf BoardsConf
	err = loadTOML(boardsConfFilePath, &boardsConf)
	if err != nil {
		fmt.Println("❌ Unable to parse file (missing or incorrectly formatted)")
		fmt.Println("❌ Missing person_column_id")
		fmt.Println("❌ Missing hours_column_id")
		validConfiguration = false
	} else {
		personColumnID := boardsConf.PersonColumnID
		hoursColumnID := boardsConf.HoursColumnID
		description := boardsConf.Description
		if personColumnID != "" && hoursColumnID != "" {
			fmt.Println("✅ File is valid")
		} else {
			if personColumnID == "" {
				fmt.Println("❌ Missing person_column_id")
				validConfiguration = false
			}
			if hoursColumnID == "" {
				fmt.Println("❌ Missing hours_column_id")
				validConfiguration = false
			}
		}
		if description != "" {
			fmt.Println("✅ Description: " + description)
		}
		// TODO add summary of data by reusing checks from create-one
	}

	if !validConfiguration {
		return WrapWithStack(err, "The boards configuration has one or more validation errors.\nRun `mlog update` to fetch the latest board configuration.")
	}
	fmt.Println("Setup complete without errors.")
	return nil
}

// TODO Detect when you are already up to date.
func update(cCtx *cli.Context) error {
	err := loadConfPaths()
	if err != nil {
		return err
	}

	boardsURL := "https://denis-engcom.github.io/mlog/boards.toml"
	boardsResponse, err := http.Get(boardsURL)
	if err != nil {
		return err
	}
	defer boardsResponse.Body.Close()

	// Download into a temporary file.
	// When everything looks good, replace real file at the end as a final step.
	boardsFile, err := os.Create(boardsConfFilePath + ".tmp")
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

	err = os.Rename(boardsConfFilePath+".tmp", boardsConfFilePath)
	if err != nil {
		return err
	}
	fmt.Printf("GET %s (%d bytes) - successful\n", boardsURL, n)
	fmt.Printf("Saved to %s\n", boardsConfFilePath)

	var boardsConf BoardsConf
	err = loadTOML(boardsConfFilePath, &boardsConf)
	if err == nil && boardsConf.Description != "" {
		fmt.Println("✅ Description: " + boardsConf.Description)
	}

	fmt.Println("Update complete without errors.")

	return nil
}

func getBoardByID(mondayAPIClient *MondayAPIClient, boardID string) error {
	logger.Debugw("getBoardByID", "boardID", boardID)
	boardIDInt, err := strconv.Atoi(boardID)
	if err != nil {
		return WithStackF("\"%d\": not a number. Exiting.", boardID)
	}
	board, err := mondayAPIClient.GetBoardByID(boardIDInt)
	if err != nil {
		return err
	}

	groups := map[string]string{}
	for _, group := range board.Groups {
		groups[group.Title] = group.ID
	}
	// Produce TOML like
	//
	// [months.2023-09]
	// board_id = 1234567890
	// [months.2023-09.days]
	// 'Fri Sep 01' = 'fri_sep_01'
	// 'Sat Sep 02' = 'sat_sep_02'
	// ...
	content := map[string]map[string]map[string]any{
		"months": {
			"yyyy-mm": {
				"board_id": board.ID,
				"name":     board.Name,
				"days":     groups,
			},
		},
	}
	return ptoml.NewEncoder(os.Stdout).Encode(&content)
}
