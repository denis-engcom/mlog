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
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	ptoml "github.com/pelletier/go-toml/v2"
)

var (
	userConfFilePath   string
	boardsConfFilePath string
	userConf           = koanf.New(".")
	boardsConf         = koanf.New(".")
	logger             *zap.SugaredLogger
)

type UserConf struct {
	APIAccessToken string `toml:"api_access_token"`
	LoggingUserID  string `toml:"logging_user_id"`
}

type BoardsConf struct {
	PersonColumnID string           `toml:"person_column_id"`
	HoursColumnID  string           `toml:"hours_column_id"`
	Description    string           `toml:"description"`
	Months         map[string]Month `toml:"months"`
}

type Month struct {
	BoardID uint64            `koanf:"board_id",toml:"board_id"`
	Days    map[string]string `koanf:"days",toml:"board_id"`
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
				Name:        "month",
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
				Name:        "get-board-by-id",
				Aliases:     []string{"gbid"},
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
			//{
			//	Name:        "get-board",
			//	Aliases:     []string{"gb"},
			//	ArgsUsage:   "<yyyy-mm>",
			//	Description: "(Admin command) get board information by month to populate boards.toml",
			//	Action: func(cCtx *cli.Context) error {
			//		err := loadConf()
			//		if err != nil {
			//			return err
			//		}
			//
			//		mondayAPIClient := NewMondayAPIClient()
			//
			//		return getBoard(mondayAPIClient, cCtx.Args().First())
			//	},
			//},
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

var unableToParseUserConfMsg = "Unable to parse user configuration file.\nRun `mlog setup` for error details."
var unableToParseBoardsConfMsg = "Unable to parse boards configuration file.\nRun `mlog setup` for error details."

func loadConf() error {
	err := loadConfPaths()
	if err != nil {
		return err
	}

	if err := userConf.Load(file.Provider(userConfFilePath), toml.Parser()); err != nil {
		return WrapWithStack(err, unableToParseUserConfMsg)
	}
	apiAccessToken := userConf.String("api_access_token")
	loggingUserID := userConf.String("logging_user_id")
	if apiAccessToken == "" || loggingUserID == "" {
		return WrapWithStack(err, unableToParseUserConfMsg)
	}

	if err := boardsConf.Load(file.Provider(boardsConfFilePath), toml.Parser()); err != nil {
		return WrapWithStack(err, unableToParseBoardsConfMsg)
	}
	personColumnID := boardsConf.MustString("person_column_id")
	hoursColumnID := boardsConf.MustString("hours_column_id")
	if personColumnID == "" || hoursColumnID == "" {
		return WrapWithStack(err, unableToParseBoardsConfMsg)
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

	boardsConf = koanf.New(".")
	if err := boardsConf.Load(file.Provider(boardsConfFilePath), toml.Parser()); err == nil {
		description := boardsConf.String("description")
		if description != "" {
			fmt.Println("✅ Description: " + description)
		}
	}

	fmt.Println("Update complete without errors.")

	return nil
}

func checkMonth(monthYYYYMM string) error {
	monthKey := fmt.Sprintf("months.%s", monthYYYYMM)
	var monthConf Month
	err := boardsConf.Unmarshal(monthKey, &monthConf)
	if err != nil {
		return WrapWithStack(err, unableToParseUserConfMsg)
	}
	if monthConf.BoardID == 0 {
		return WithStackF("%q: month not found in boards configuration. Exiting.", monthYYYYMM)
	}

	monthMap := map[string]interface{}{
		"board_id": monthConf.BoardID,
		"days":     monthConf.Days,
	}
	monthBytes, err := toml.Parser().Marshal(monthMap)
	if err != nil {
		return WrapWithStack(err, unableToParseUserConfMsg)
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

func getBoardIDForMonth(month string) int {
	key := fmt.Sprintf("months.%s.board_id", month)
	return boardsConf.Int(key)
}

func getBoard(mondayAPIClient *MondayAPIClient, monthYYYYMM string) error {
	boardID := getBoardIDForMonth(monthYYYYMM)
	if boardID == 0 {
		return WithStackF("\"months.%s.board_id\": not found in boards configuration. Exiting.", monthYYYYMM)
	}
	logger.Debugw("getBoardByID", "month", monthYYYYMM, "boardID", boardID)
	board, err := mondayAPIClient.GetBoardByID(boardID)
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
			monthYYYYMM: {
				"board_id": board.ID,
				"name":     board.Name,
				"days":     groups,
			},
		},
	}
	return ptoml.NewEncoder(os.Stdout).Encode(&content)
}

func getGroupIDForDay(month, day string) string {
	key := fmt.Sprintf("months.%s.days.%s", month, day)
	return boardsConf.String(key)
}

func createOne(mondayAPIClient *MondayAPIClient, dayYYYYMMDD, itemName, hours string) error {
	if len(dayYYYYMMDD) != 10 {
		return WithStackF("%q: provided day is not in format yyyy-mm-dd. Exiting.", dayYYYYMMDD)
	}
	month := dayYYYYMMDD[0:7]
	boardID := getBoardIDForMonth(month)
	if boardID == 0 {
		return WithStackF("\"months.%s.board_id\": not found in boards configuration. Exiting.", month)
	}
	day := dayYYYYMMDD[7:10]
	groupID := getGroupIDForDay(month, day)
	if groupID == "" {
		return WithStackF("\"month.%s.days.%s\": not found in boards configuration. Exiting.", month, day)
	}
	logger.Debugw("createOne", "day", dayYYYYMMDD, "boardID", boardID, "groupID", groupID, "itemName", itemName, "hours", hours)

	res, err := mondayAPIClient.CreateLogItem(boardID, groupID, itemName, hours)
	if err != nil {
		return err
	}
	fmt.Printf("https://magicboard.monday.com/boards/%d/pulses/%s\n", boardID, res.Create_Item.ID)
	return nil
}
