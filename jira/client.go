package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"io/ioutil"
	"net/http"
	"net/http/httputil"

	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/urlutil"
)

const (
	apiEndPoint     = "/rest/api/2/issue/"
	commentEndPoint = "/comment"
)

// Client ...
type Client struct {
	token   string
	client  *http.Client
	headers map[string]string
	baseURL string
}

// Comment ...
type Comment struct {
	Content string
	IssuKey string
}

type response struct {
	issueKey string
	err      error
}

func (resp response) String() string {
	respValue := map[bool]string{true: colorstring.Green("SUCCES"), false: colorstring.Red("FAILED")}[resp.err == nil]
	return fmt.Sprintf("Posting comment to - %s - : %s", resp.issueKey, respValue)
}

// -------------------------------------
// -- Public methods

// NewClient ...
func NewClient(token, baseURL string) *Client {
	return &Client{
		token:  token,
		client: &http.Client{},
		headers: map[string]string{
			"Authorization": `Basic ` + token,
			"Content-Type":  "application/json",
		},
		baseURL: baseURL,
	}
}

// PostIssueComments ...
func (client *Client) PostIssueComments(comments []Comment) error {
	if len(comments) == 0 {
		return fmt.Errorf("no comment has been added")
	}

	ch := make(chan response, len(comments))
	for _, comment := range comments {
		go client.postIssueComment(comment, ch)
	}

	counter := 0
	var respErrors []response
	for resp := range ch {
		counter++
		log.Printf(resp.String())

		if resp.err != nil {
			respErrors = append(respErrors, resp)
		}

		if counter >= len(comments) {
			break
		}
	}

	if len(respErrors) > 0 {
		fmt.Println()
		log.Infof("Errors during posting comments:")

		for _, respErr := range respErrors {
			log.Warnf("Error during posting comment to - %s - : %s", respErr.issueKey, respErr.err.Error())
		}

		fmt.Println()
	}

	return map[bool]error{true: fmt.Errorf("some comments were failed to be posted at Jira")}[len(respErrors) > 0]
}

// -------------------------------------
// -- Private methods

func (client *Client) postIssueComment(comment Comment, ch chan response) {
	t := strings.TrimSpace(comment.IssuKey)
	requestURL, err := urlutil.Join(client.baseURL, apiEndPoint, t, commentEndPoint)
	if err != nil {
		ch <- response{comment.IssuKey, err}
		return
	}

	fields := map[string]interface{}{"body": comment.Content}

	request, err := createRequest(http.MethodPost, requestURL, client.headers, fields)
	if err != nil {
		ch <- response{comment.IssuKey, err}
		return
	}

	requestBytes, err := httputil.DumpRequest(request, true)
	if err != nil {
		ch <- response{comment.IssuKey, err}
		return
	}
	log.Debugf("Request: %v", string(requestBytes))

	// Perform request
	_, body, err := client.performRequest(request, nil)
	log.Debugf("Body: %s", string(body))
	ch <- response{comment.IssuKey, err}
}

func createRequest(requestMethod string, url string, headers map[string]string, fields map[string]interface{}) (*http.Request, error) {
	var jsonContent []byte

	if len(fields) > 0 {
		var err error
		if jsonContent, err = json.Marshal(fields); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(requestMethod, url, bytes.NewBuffer(jsonContent))
	if err != nil {
		return nil, err
	}

	addHeaders(req, headers)
	return req, nil
}

func (client *Client) performRequest(req *http.Request, requestResponse interface{}) (interface{}, []byte, error) {
	response, err := client.client.Do(req)
	if err != nil {
		// On error, any Response can be ignored
		return nil, nil, fmt.Errorf("failed to perform request, error: %s", err)
	}

	// The client must close the response body when finished with it
	defer func() {
		if cerr := response.Body.Close(); cerr != nil {
			log.Warnf("Failed to close response body, error: %s", cerr)
		}
	}()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body, error: %s", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode > http.StatusMultipleChoices {
		return nil, nil, fmt.Errorf("Response status: %d - Body: %s", response.StatusCode, string(body))
	}

	// Parse JSON body
	if requestResponse != nil {
		if err := json.Unmarshal([]byte(body), &requestResponse); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal response (%s), error: %s", body, err)
		}

		logDebugPretty(&requestResponse)
	}
	return requestResponse, body, nil
}

func addHeaders(req *http.Request, headers map[string]string) {
	for key, value := range headers {
		req.Header.Set(key, value)
	}
}

func logDebugPretty(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println("error:", err)
	}

	log.Debugf("Response: %+v\n", string(b))
}
