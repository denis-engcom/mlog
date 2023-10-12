package main

import (
	"context"
	"fmt"
	"github.com/hasura/go-graphql-client"
	"net/http"
	"strconv"
)

// JSONEncodedString avoids a type mismatch in the GraphQL library when setting a JSON-encoded string property.
type JSONEncodedString string

func (_ JSONEncodedString) GetGraphQLType() string { return "JSON" }

// CompareValue avoids a type mismatch in the GraphQL library when setting a string meant for type CompareValue.
type CompareValue string

func (_ CompareValue) GetGraphQLType() string { return "CompareValue" }

type MondayAPIClient struct {
	client         *graphql.Client
	loggingUserID  string
	personColumnID string
	hoursColumnID  string
}

// NewMondayAPIClient forms the client with common information needed during Monday API calls.
func NewMondayAPIClient(apiAccessToken, loggingUserID, personColumnID, hoursColumnID string) *MondayAPIClient {
	client := graphql.NewClient("https://api.monday.com/v2/", nil).
		//WithDebug(true).
		WithRequestModifier(func(req *http.Request) {
			req.Header.Add("Authorization", apiAccessToken)
			// The latest version of the Monday API won't be used by default until January 2024.
			req.Header.Add("API-Version", "2023-10")
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
func (m *MondayAPIClient) GetBoardByID(boardID string) (*Board, error) {
	vars := map[string]any{
		"board_ids": []graphql.ID{graphql.ToID(boardID)},
	}
	var gbq GetBoardsQuery
	err := m.client.Query(context.TODO(), &gbq, vars)
	if err != nil {
		return nil, WrapWithStackF(err,
			"A problem occurred when contacting monday.com. Exiting.")
	}
	return &gbq.Boards[0], nil
}

//	query {
//	  boards(ids: 5064273451) {
//	    id
//	    name
//	    items_page(limit: 100, query_params: {rules: [{column_id: "person-column", compare_value: ["person-" + "logging-user-id"]}]}) {
//	      cursor
//	      items {
//	        id
//	        name
//	        group { title }
//	        column_values(ids: "hours-column") { text }
//	      }
//	    }
//	  }
//	}
type BoardItem struct {
	ID    string
	Name  string
	Group struct {
		Title string
	}
	Column_Values []struct {
		Text string
	} `graphql:"column_values(ids: $hours_column_id)"`
}

type BoardWithItems struct {
	ID         string
	Name       string
	Items_Page struct {
		Cursor string
		Items  []BoardItem
	} `graphql:"items_page(limit: 100, query_params: { rules: { column_id: $person_column_id, compare_value: $logging_user_id} })"`
}

type GetBoardItemsQuery struct {
	Boards []BoardWithItems `graphql:"boards(ids: $board_ids)"`
}

// GetBoardItems calls the Monday API "boards" query and returns the logging user's items.
func (m *MondayAPIClient) GetBoardItems(boardID string) (*BoardWithItems, error) {
	vars := map[string]any{
		"board_ids":        []graphql.ID{graphql.ToID(boardID)},
		"logging_user_id":  CompareValue("person-" + m.loggingUserID),
		"hours_column_id":  []string{m.hoursColumnID},
		"person_column_id": graphql.ToID(m.personColumnID),
	}
	var gbiq GetBoardItemsQuery
	err := m.client.Query(context.TODO(), &gbiq, vars)
	if err != nil {
		return nil, WrapWithStackF(err,
			"A problem occurred when contacting monday.com. Exiting.")
	}
	return &gbiq.Boards[0], nil
}

type CreateLogItemMutate struct {
	Create_Item struct {
		Relative_Link string
	} `graphql:"create_item (board_id: $board_id, group_id: $group_id, item_name: $item_name, column_values: $column_values)"`
}

// CreateLogItem calls the Monday api "create_item" mutation.
func (m *MondayAPIClient) CreateLogItem(boardID int, groupID, itemName, hours string) (*CreateLogItemMutate, error) {
	// Validating it's a float, but can still make direct use of the string value in the request.
	_, err := strconv.ParseFloat(hours, 64)
	if err != nil {
		return nil, WrapWithStackF(err, "hours = %s (third arg): unable to parse hours as a number. Exiting.", hours)
	}
	// Person and Hours key-value pairs have to be provided together as a JSON-encoded string property.
	columnValues := fmt.Sprintf(`{"%s":"%s","%s":%s}`, m.personColumnID, m.loggingUserID, m.hoursColumnID, hours)

	vars := map[string]any{
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

//	query {
//		items(ids: [5244659133]) {
//			relative_link
//		}
//	}
type PulseRelativeLink struct {
	Relative_Link string
}

type GetPulseRelativeLinkQuery struct {
	PRL []PulseRelativeLink `graphql:"items(ids: $pulse_ids)"`
}

func (m *MondayAPIClient) GetPulseRelativeLink(pulseID string) (*PulseRelativeLink, error) {
	vars := map[string]any{
		"pulse_ids": []graphql.ID{graphql.ToID(pulseID)},
	}
	var gprlq GetPulseRelativeLinkQuery
	err := m.client.Query(context.TODO(), &gprlq, vars)
	if err != nil {
		return nil, WrapWithStackF(err,
			"A problem occurred when contacting monday.com. Exiting.")
	}
	return &gprlq.PRL[0], nil
}
