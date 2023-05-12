package main

import (
	"context"
	"fmt"
	"github.com/hasura/go-graphql-client"
	"net/http"
	"strconv"
)

type MondayAPIClient struct {
	client         *graphql.Client
	loggingUserID  string
	personColumnID string
	hoursColumnID  string
}

func NewMondayAPIClient(apiAccessToken, loggingUserID, personColumnID, hoursColumnID string) *MondayAPIClient {
	client := graphql.NewClient("https://api.monday.com/v2/", nil).
		WithRequestModifier(func(req *http.Request) {
			req.Header.Add("Authorization", apiAccessToken)
		})
	return &MondayAPIClient{
		client:         client,
		loggingUserID:  loggingUserID,
		personColumnID: personColumnID,
		hoursColumnID:  hoursColumnID,
	}
}

type Board struct {
	ID      string
	Name    string
	Columns []struct {
		ID    string
		Title string
	}
	Groups []struct {
		ID    string
		Title string
	}
}

type GetBoardsQuery struct {
	Boards []Board `graphql:"boards(ids: $board_ids)"`
}

func (m *MondayAPIClient) GetBoard(boardID string) (*Board, error) {
	boardIDInt, err := strconv.Atoi(boardID)
	if err != nil {
		return nil, err
	}
	vars := map[string]interface{}{
		"board_ids": []int{boardIDInt},
	}
	var gbq GetBoardsQuery
	err = m.client.Query(context.TODO(), &gbq, vars)
	if err != nil {
		return nil, err
	}
	return &gbq.Boards[0], nil
}

type JSONEncodedString string

func (_ JSONEncodedString) GetGraphQLType() string { return "JSON" }

type CreateLogItemMutate struct {
	Create_Item struct {
		ID string
	} `graphql:"create_item (board_id: $board_id, group_id: $group_id, item_name: $item_name, column_values: $column_values)"`
}

func (m *MondayAPIClient) CreateLogItem(boardID, groupID, itemName, hours string) (*CreateLogItemMutate, error) {
	boardIDInt, err := strconv.Atoi(boardID)
	if err != nil {
		return nil, err
	}
	// Validating it's a float, but will plug in the string value in the request...
	_, err = strconv.ParseFloat(hours, 64)
	if err != nil {
		return nil, err
	}
	// Quote values, except hours to interpret as number
	columnValues := fmt.Sprintf(`{"%s":"%s","%s":%s}`, m.personColumnID, m.loggingUserID, m.hoursColumnID, hours)
	//logger.Info(columnValuesStr)

	vars := map[string]interface{}{
		"board_id":      boardIDInt,
		"group_id":      groupID,
		"item_name":     itemName,
		"column_values": JSONEncodedString(columnValues),
	}
	var update CreateLogItemMutate
	err = m.client.Mutate(context.TODO(), &update, vars)
	if err != nil {
		return nil, err
	}
	return &update, nil
}
