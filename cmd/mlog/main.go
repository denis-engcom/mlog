package main

import (
	"cmp"
	_ "embed"
	"fmt"
	"io"
	// "log"
	"net/http"
	"os"
	"slices"
	"strconv"

	"github.com/adrg/xdg"
	"github.com/cheynewallace/tabby"
	"github.com/pelletier/go-toml/v2"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
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
		Version:              "0.2.2",
		HideHelpCommand:      true,
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "debug", Aliases: []string{"d"}},
		},
		Commands: cli.Commands{
			{
				Name:        "setup",
				Description: "Setup configuration files needed by the other mlog commands",
				Action:      cliSetup,
			},
			{
				Name:        "update",
				Aliases:     []string{"u"},
				Description: "Fetch the latest boards.toml configuration",
				Action:      cliUpdate,
			},
			{
				Name:        "get-board-items",
				Aliases:     []string{"gbi"},
				ArgsUsage:   "<yyyy-mm>",
				Description: "Get the logging user's items from the given month's board",
				Action:      cliGetBoardItems,
			},
			{
				Name:        "get-board-item-summary",
				Aliases:     []string{"gbis"},
				ArgsUsage:   "<yyyy-mm>",
				Description: "Get the logging user's item summary from the given month's board",
				Action:      cliGetBoardItemSummary,
			},
			{
				Name:        "create-one",
				Aliases:     []string{"co"},
				ArgsUsage:   "<yyyy-mm-dd> <item-description> <hours>",
				Description: "Create one log entry with info provided on the command line",
				Action:      cliCreateOne,
			},
			{
				Name:        "pulse-link",
				Aliases:     []string{"pl"},
				ArgsUsage:   "<pulse-id>",
				Description: "Print the pulse link for a given pulse ID",
				Action:      cliPulseLink,
			},
			{
				Name:        "admin-get-board-by-id",
				Aliases:     []string{"agbid"},
				ArgsUsage:   "<board-id>",
				Description: "(Admin command) get board information by board-id to populate boards.toml",
				Action:      cliGetBoardByID,
			},
		},
		// Adapt error handling to...
		// * printing stack traces during debug mode
		// * using errors.As to get ExitCoder at any level for printing
		ExitErrHandler: customErrorHandler,
	}

	app.Run(os.Args)
}

var (
	msgMonthBoardIDNotFound    = "\"months.%s.board_id\": not found in boards configuration. Exiting."
	msgDayGroupNotFound        = "\"month.%s.days.%s\": not found in boards configuration. Exiting."
	msgUnableToParseUserConf   = "Unable to parse user configuration file.\nRun `mlog setup` for error details."
	msgUnableToParseBoardsConf = "Unable to parse boards configuration file.\nRun `mlog setup` for error details."
)

func loadConf() (*UserConf, *BoardsConf, error) {
	err := loadConfPaths()
	if err != nil {
		return nil, nil, err
	}

	var userConf UserConf
	err = loadTOML(userConfFilePath, &userConf)
	if err != nil {
		return nil, nil, WrapWithStack(err, msgUnableToParseUserConf)
	}
	if userConf.APIAccessToken == "" || userConf.LoggingUserID == "" {
		return nil, nil, WrapWithStack(err, msgUnableToParseUserConf)
	}

	var boardsConf BoardsConf
	err = loadTOML(boardsConfFilePath, &boardsConf)
	if err != nil {
		return nil, nil, WrapWithStack(err, msgUnableToParseBoardsConf)
	}
	if boardsConf.PersonColumnID == "" || boardsConf.HoursColumnID == "" {
		return nil, nil, WrapWithStack(err, msgUnableToParseBoardsConf)
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
	return toml.NewDecoder(file).Decode(obj)
}

// TODO Improve setup by
//  1. asking for access token
//  2. Calling the "me" API to get the "logging user ID"

//	query {
//		me {
//	 		id
//	 		name
//	 		url
//		}
//	}
func cliSetup(cCtx *cli.Context) error {
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
func cliUpdate(cCtx *cli.Context) error {
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

func cliGetBoardItems(cCtx *cli.Context) error {
	// TODO Day version of this route
	// mlog get-board-items 2023-09-01
	// For now, rely on grepping of group names
	userConf, boardsConf, err := loadConf()
	if err != nil {
		return err
	}

	mondayAPIClient := NewMondayAPIClient(
		userConf.APIAccessToken,
		userConf.LoggingUserID,
		boardsConf.PersonColumnID,
		boardsConf.HoursColumnID)

	monthYYYYMM := cCtx.Args().First()
	month := boardsConf.Months[monthYYYYMM]
	if month == nil || month.BoardID == "" {
		return WithStackF(msgMonthBoardIDNotFound, monthYYYYMM)
	}

	logger.Debugw("GetBoardItems", "boardID", month.BoardID)
	boardWithItems, err := mondayAPIClient.GetBoardItems(month.BoardID)
	if err != nil {
		return err
	}

	items := boardWithItems.Items_Page.Items
	slices.SortFunc(items, func(a, b BoardItem) int {
		aGroup := a.Group.Title
		bGroup := b.Group.Title
		if len(aGroup) == 10 && len(bGroup) == 10 {
			if aGroup[8] > bGroup[8] {
				return 1
			} else if aGroup[8] < bGroup[8] {
				return -1
			}
			if aGroup[9] > bGroup[9] {
				return 1
			} else if aGroup[9] < bGroup[9] {
				return -1
			}
		}

		return cmp.Compare(a.ID, b.ID)
	})

	//return json.NewEncoder(os.Stdout).Encode(items.Items)
	table := tabby.New()
	table.AddHeader("GROUP", "HOURS", "DESCRIPTION", "PULSE ID")
	for _, item := range boardWithItems.Items_Page.Items {
		table.AddLine(item.Group.Title, item.Column_Values[0].Text, item.Name, item.ID)
	}
	table.Print()
	return nil
}

func cliGetBoardItemSummary(cCtx *cli.Context) error {
	userConf, boardsConf, err := loadConf()
	if err != nil {
		return err
	}

	mondayAPIClient := NewMondayAPIClient(
		userConf.APIAccessToken,
		userConf.LoggingUserID,
		boardsConf.PersonColumnID,
		boardsConf.HoursColumnID)

	monthYYYYMM := cCtx.Args().First()
	month := boardsConf.Months[monthYYYYMM]
	if month == nil || month.BoardID == "" {
		return WithStackF(msgMonthBoardIDNotFound, monthYYYYMM)
	}

	logger.Debugw("GetBoardItems", "boardID", month.BoardID)
	boardWithItems, err := mondayAPIClient.GetBoardItems(month.BoardID)
	if err != nil {
		return err
	}

	type GroupData struct {
		Group      string
		TotalHours float64
		PulseCount int
	}
	groupMap := map[string]GroupData{}
	for _, item := range boardWithItems.Items_Page.Items {
		hours, err := strconv.ParseFloat(item.Column_Values[0].Text, 64)
		if err != nil {
			return WrapWithStackF(err, "hours = %s (pulse_id = %s): not a number. Exiting.",
				item.Column_Values[0].Text,
				item.ID)
		}
		gd := groupMap[item.Group.Title]
		gd.TotalHours += hours
		gd.PulseCount += 1
		groupMap[item.Group.Title] = gd
	}
	groups := make([]GroupData, 0, len(groupMap))
	for gKey, gVal := range groupMap {
		gVal.Group = gKey
		groups = append(groups, gVal)
	}

	slices.SortFunc(groups, func(a, b GroupData) int {
		aGroup := a.Group
		bGroup := b.Group
		if len(aGroup) == 10 && len(bGroup) == 10 {
			if aGroup[8] > bGroup[8] {
				return 1
			} else if aGroup[8] < bGroup[8] {
				return -1
			}
			if aGroup[9] > bGroup[9] {
				return 1
			} else if aGroup[9] < bGroup[9] {
				return -1
			}
		}

		return cmp.Compare(aGroup, bGroup)
	})

	table := tabby.New()
	table.AddHeader("GROUP", "TOTAL HOURS", "PULSE COUNT")
	for _, group := range groups {
		table.AddLine(group.Group, group.TotalHours, group.PulseCount)
	}
	table.Print()
	return nil
}

func cliCreateOne(cCtx *cli.Context) error {
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

	return createOne(mondayAPIClient, boardsConf, dayYYYYMMDD, itemName, hours)
}

func createOne(mondayAPIClient *MondayAPIClient, boardsConf *BoardsConf, dayYYYYMMDD, itemName, hours string) error {
	if len(dayYYYYMMDD) != 10 {
		return WithStackF("day = %s (first arg): provided day is not in format yyyy-mm-dd. Exiting.", dayYYYYMMDD)
	}

	monthYYYYMM := dayYYYYMMDD[0:7]
	if len(boardsConf.Months) == 0 {
		return WithStackF(msgMonthBoardIDNotFound, monthYYYYMM)
	}
	month := boardsConf.Months[monthYYYYMM]
	if month == nil || month.BoardID == "" {
		return WithStackF(msgMonthBoardIDNotFound, monthYYYYMM)
	}
	boardIDInt, err := strconv.Atoi(month.BoardID)
	if err != nil {
		return WrapWithStackF(err, "\"months.%s.board_id\": not a number. Exiting.", monthYYYYMM)
	}

	dayDD := dayYYYYMMDD[7:10]
	if len(month.Days) == 0 {
		return WithStackF(msgDayGroupNotFound, monthYYYYMM, dayDD)
	}
	dayGroupID := month.Days[dayDD]
	if dayGroupID == "" {
		return WithStackF(msgDayGroupNotFound, monthYYYYMM, dayDD)
	}
	logger.Debugw("CreateLogItem", "day", dayYYYYMMDD, "boardID", boardIDInt, "groupID", dayGroupID, "itemName", itemName, "hours", hours)

	res, err := mondayAPIClient.CreateLogItem(boardIDInt, dayGroupID, itemName, hours)
	if err != nil {
		return err
	}
	fmt.Printf("https://magicboard.monday.com%s\n", res.Create_Item.Relative_Link)
	return nil
}

func cliPulseLink(cCtx *cli.Context) error {
	userConf, boardsConf, err := loadConf()
	if err != nil {
		return err
	}

	mondayAPIClient := NewMondayAPIClient(
		userConf.APIAccessToken,
		userConf.LoggingUserID,
		boardsConf.PersonColumnID,
		boardsConf.HoursColumnID)

	pulseID := cCtx.Args().First()

	logger.Debugw("GetPulseRelativeLink", "pulseID", pulseID)
	prl, err := mondayAPIClient.GetPulseRelativeLink(pulseID)
	if err != nil {
		return err
	}

	fmt.Printf("https://magicboard.monday.com%s\n", prl.Relative_Link)
	return nil
}

func cliGetBoardByID(cCtx *cli.Context) error {
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
}

func getBoardByID(mondayAPIClient *MondayAPIClient, boardID string) error {
	logger.Debugw("GetBoardByID", "boardID", boardID)
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
			"yyyy-mm": {
				"board_id": board.ID,
				"name":     board.Name,
				"days":     groups,
			},
		},
	}
	return toml.NewEncoder(os.Stdout).Encode(&content)
}
