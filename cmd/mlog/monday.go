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

// NewMondayAPIClient forms the client with common information needed during Monday API calls.
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

// GetBoardByID calls the Monday API "boards" query with a single board and returns it.
func (m *MondayAPIClient) GetBoardByID(boardID int) (*Board, error) {
	vars := map[string]interface{}{
		"board_ids": []int{boardID},
	}
	var gbq GetBoardsQuery
	err := m.client.Query(context.TODO(), &gbq, vars)
	if err != nil {
		return nil, WrapWithStackF(err,
			"A problem occurred when contacting monday.com. Exiting.")
	}
	return &gbq.Boards[0], nil
}

// JSONEncodedString avoids a type mismatch in the GraphQL library when setting a JSON-encoded string property.
type JSONEncodedString string

func (_ JSONEncodedString) GetGraphQLType() string { return "JSON" }

type CreateLogItemMutate struct {
	Create_Item struct {
		ID string
	} `graphql:"create_item (board_id: $board_id, group_id: $group_id, item_name: $item_name, column_values: $column_values)"`
}

// CreateLogItem calls the Monday api "create_item" mutation.
func (m *MondayAPIClient) CreateLogItem(boardID string, groupID, itemName, hours string) (*CreateLogItemMutate, error) {
	// Validating it's a float, but can still make direct use of the string value in the request.
	_, err := strconv.ParseFloat(hours, 64)
	if err != nil {
		return nil, WrapWithStackF(err, "%q: unable to parse hours as a number. Exiting.", hours)
	}
	// Person and Hours key-value pairs have to be provided together as a JSON-encoded string property.
	columnValues := fmt.Sprintf(`{"%s":"%s","%s":%s}`, m.personColumnID, m.loggingUserID, m.hoursColumnID, hours)

	vars := map[string]interface{}{
		"board_id":      boardID,
		"group_id":      groupID,
		"item_name":     itemName,
		"column_values": JSONEncodedString(columnValues),
	}
	var update CreateLogItemMutate
	err = m.client.Mutate(context.TODO(), &update, vars)
	if err != nil {
		return nil, WrapWithStackF(err,
			"A problem occurred when contacting monday.com. Please verify on monday.com whether a log entry was created or not. Exiting.")
	}
	return &update, nil
}
