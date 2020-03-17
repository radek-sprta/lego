package clouddns

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

const apiBaseURL = "https://admin.vshosting.cloud/clouddns"
const loginURL = "https://admin.vshosting.cloud/api/public/auth/login"

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type authorization struct {
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

type Client struct {
	AccessToken string
	ClientID    string
	Email       string
	Password    string
	TTL         int
	HTTPClient  *http.Client
}

type record struct {
	DomainID string `json:"domainId,omitempty"`
	Name     string `json:"name,omitempty"`
	Value    string `json:"value,omitempty"`
	Type     string `json:"type,omitempty"`
}

type searchBlock struct {
	Name     string
	Operator string
	Value    string
}

func NewClient(clientID string, email string, password string, ttl int) *Client {
	return &Client{
		AccessToken: "",
		ClientID:    clientID,
		Email:       email,
		Password:    password,
		TTL:         ttl,
		HTTPClient:  &http.Client{},
	}
}

func (c *Client) AddRecord(zone, recordName, recordValue string) error {
	domainID, err := c.getDomainID(zone)
	if err != nil {
		return err
	}

	err = c.addTxtRecord(domainID, recordName, recordValue)
	if err != nil {
		return err
	}

	err = c.publishRecords(domainID)
	return err
}

func (c *Client) addTxtRecord(domainID string, recordName string, recordValue string) error {
	txtRecord := record{DomainID: domainID, Name: recordName, Value: recordValue, Type: "TXT"}
	body, err := json.Marshal(txtRecord)
	if err != nil {
		return err
	}

	_, err = c.doAPIRequest(http.MethodPost, "record-txt", bytes.NewReader(body))
	return err
}

func (c *Client) DeleteRecord(zone, recordName string) error {
	domainID, err := c.getDomainID(zone)
	if err != nil {
		return err
	}

	recordID, err := c.getRecordID(domainID, recordName)
	if err != nil {
		return err
	}

	err = c.deleteRecordByID(recordID)
	if err != nil {
		return err
	}

	err = c.publishRecords(domainID)
	return err
}

func (c *Client) deleteRecordByID(recordID string) error {
	endpoint := fmt.Sprintf("record/%s", recordID)
	_, err := c.doAPIRequest(http.MethodDelete, endpoint, nil)
	return err
}

func (c *Client) doAPIRequest(method, endpoint string, body io.Reader) ([]byte, error) {
	if c.AccessToken == "" {
		err := c.login()
		if err != nil {
			return nil, err
		}
	}

	url := fmt.Sprintf("%s/%s", apiBaseURL, endpoint)

	req, err := c.newRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	content, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readError(req, resp)
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (c *Client) getDomainID(zone string) (string, error) {
	searchClient := searchBlock{Name: "clientId", Operator: "eq", Value: c.ClientID}
	searchDomain := searchBlock{Name: "domainName", Operator: "eq", Value: zone}
	searchBody := map[string][]searchBlock{"search": {searchClient, searchDomain}}

	body, err := json.Marshal(searchBody)
	if err != nil {
		return "", err
	}

	resp, err := c.doAPIRequest(http.MethodPost, "domain/search", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return "", err
	}

	// Let's dig for the .["items"][0]["id"] path
	items := result["items"].([]interface{})
	domainDetails := items[0].(map[string]interface{})
	domainID := domainDetails["id"].(string)

	return domainID, nil
}

func (c *Client) getRecordID(domainID, recordName string) (string, error) {
	endpoint := fmt.Sprintf("domain/%s", domainID)
	resp, err := c.doAPIRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return "", err
	}

	recordID := ""
	entries := result["lastDomainRecordList"].([]interface{})
	for _, entry := range entries {
		entryMap := entry.(map[string]interface{})
		if entryMap["name"] == recordName && entryMap["type"] == "TXT" {
			recordID = entryMap["id"].(string)
		}
	}
	return recordID, nil
}

func (c *Client) login() error {
	reqData := authorization{Email: c.Email, Password: c.Password}
	body, err := json.Marshal(reqData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	content, err := c.doRequest(req)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	err = json.Unmarshal(content, &result)
	if err != nil {
		return err
	}

	authBlock := result["auth"].(map[string]interface{})
	c.AccessToken = authBlock["accessToken"].(string)

	return nil
}

func (c *Client) newRequest(method, reqURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))

	return req, nil
}

func (c *Client) publishRecords(domainID string) error {
	soaTTL := map[string]int{"soaTtl": c.TTL}
	body, err := json.Marshal(soaTTL)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("domain/%s/publish", domainID)
	_, err = c.doAPIRequest(http.MethodPut, endpoint, bytes.NewReader(body))
	return err
}

func readError(req *http.Request, resp *http.Response) error {
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.New(toUnreadableBodyMessage(req, content))
	}

	var errInfo apiError
	err = json.Unmarshal(content, &errInfo)
	if err != nil {
		return fmt.Errorf("apiError unmarshaling error: %v: %s", err, toUnreadableBodyMessage(req, content))
	}

	return fmt.Errorf("HTTP %d: %s: %s", resp.StatusCode, errInfo.Code, errInfo.Message)
}

func toUnreadableBodyMessage(req *http.Request, rawBody []byte) string {
	return fmt.Sprintf("the request %s sent a response with a body which is an invalid format: %q", req.URL, string(rawBody))
}
