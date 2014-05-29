package client

import (
	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"errors"
	"fmt"
	bigquery "github.com/Dailyburn/google-api-go-client-bigquery/bigquery/v2"
	"io/ioutil"
	"net/http"
	"strconv"
)

const AuthUrl = "https://accounts.google.com/o/oauth2/auth"
const TokenUrl = "https://accounts.google.com/o/oauth2/token"

type Client struct {
	accountEmailAddress string
	userAccountClientId string
	clientSecret        string
	pemPath             string
	token               *oauth.Token
	service             *bigquery.Service
}

// Instantiate a new client with the given params and return a reference to it
func New(pemPath, serviceAccountEmailAddress, serviceUserAccountClientId, clientSecret string) *Client {
	return &Client{pemPath: pemPath, clientSecret: clientSecret, accountEmailAddress: serviceAccountEmailAddress, userAccountClientId: serviceUserAccountClientId}
}

func (c *Client) connect() (*bigquery.Service, error) {
	if c.token != nil {
		fmt.Println("token expired", c.token.Expired())
		fmt.Println("token expiry", c.token.Expiry)

		if !c.token.Expired() && c.service != nil {
			fmt.Println("REUSE SERVICE")
			return c.service, nil
		}
	} else {

	}

	// generate auth token and create service object
	authScope := bigquery.BigqueryScope
	pemKeyBytes, err := ioutil.ReadFile(c.pemPath)
	if err != nil {
		panic(err)
	}

	t := jwt.NewToken(c.accountEmailAddress, bigquery.BigqueryScope, pemKeyBytes)

	httpClient := &http.Client{}
	token, err := t.Assert(httpClient)
	if err != nil {
		return nil, err
	}

	c.token = token

	config := &oauth.Config{
		ClientId:     c.userAccountClientId,
		ClientSecret: c.clientSecret,
		Scope:        authScope,
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://accounts.google.com/o/oauth2/token",
	}

	transport := &oauth.Transport{
		Token:  token,
		Config: config,
	}

	client := transport.Client()

	service, err := bigquery.New(client)
	if err != nil {
		return nil, err
	}

	c.service = service
	return service, nil
}

func (c *Client) InsertRow(projectId, datasetId, tableId string, rowData map[string]interface{}) error {
	service, err := c.connect()
	if err != nil {
		return err
	}

	rows := []*bigquery.TableDataInsertAllRequestRows{
		{
			Json: rowData,
		},
	}

	insertRequest := &bigquery.TableDataInsertAllRequest{Rows: rows}

	result, err := service.Tabledata.InsertAll(projectId, datasetId, tableId, insertRequest).Do()
	if err != nil {
		fmt.Println("Error inserting row: ", err)
		return err
	}

	if len(result.InsertErrors) > 0 {
		return errors.New("Error inserting row")
	}

	return nil
}

// SyncQuery executes an arbitrary query string and returns the result synchronously (unless the response takes longer than the provided timeout)
func (c *Client) SyncQuery(dataset, project, queryStr string, maxResults int64) ([][]interface{}, error) {
	service, err := c.connect()
	if err != nil {
		return nil, err
	}

	datasetRef := &bigquery.DatasetReference{
		DatasetId: dataset,
		ProjectId: project,
	}

	query := &bigquery.QueryRequest{
		DefaultDataset: datasetRef,
		MaxResults:     maxResults,
		Kind:           "json",
		Query:          queryStr,
	}

	results, err := service.Jobs.Query(project, query).Do()
	if err != nil {
		fmt.Println("Query Error: ", err)
		return nil, err
	}

	// credit to https://github.com/getlantern/statshub for the row building approach
	numRows := int(results.TotalRows)
	if numRows > int(maxResults) {
		numRows = int(maxResults)
	}

	rows := make([][]interface{}, numRows)
	for r := 0; r < int(numRows); r++ {
		numColumns := len(results.Schema.Fields)
		dataRow := results.Rows[r]
		row := make([]interface{}, numColumns)
		for c := 0; c < numColumns; c++ {
			row[c] = dataRow.F[c].V
		}
		rows[r] = row
	}

	return rows, nil
}

// Count loads the row count for the provided dataset.tablename
func (c *Client) Count(dataset, project, datasetTable string) int64 {
	qstr := fmt.Sprintf("select count(*) from [%s]", datasetTable)
	res, err := c.SyncQuery(dataset, project, qstr, 1)
	if err == nil {
		if len(res) > 0 {
			val, _ := strconv.ParseInt(res[0][0].(string), 10, 64)
			return val
		}
	}
	return 0
}
