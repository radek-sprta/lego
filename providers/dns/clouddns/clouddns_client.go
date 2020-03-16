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
	Email string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

type cloudDnsClient struct {
    AccessToken string
    ClientId string
	Email string
	Password string
    TTL     int
	HTTPClient         *http.Client
}

type record struct {
	DomainId string `json:"domainId,omitempty"`
	Name     string `json:"name,omitempty"`
	Value    string `json:"value,omitempty"`
	Type     string `json:"type,omitempty"`
}

type searchBlock struct {
    Name     string
    Operator string
    Value    string
}

func NewCloudDnsClient(clientId string, email string, password string, ttl int) (*cloudDnsClient) {
	return &cloudDnsClient{
        AccessToken: "",
		ClientId:    clientId,
		Email:       email,
		Password:    password,
        TTL:         ttl,
		HTTPClient:  &http.Client{},
	}
}

func (c *cloudDnsClient) AddRecord(zone, recordName, recordValue string) (error) {
    domainId, err := c.getDomainId(zone)

    err = c.addTxtRecord(domainId, recordName, recordValue)
    if err != nil {
        return err
    }

    err = c.publishRecords(domainId)
    return err
}

func (c *cloudDnsClient) addTxtRecord(domainId string,recordName string, recordValue string) (error) {
    txtRecord := record{DomainId: domainId, Name: recordName, Value: recordValue, Type: "TXT"}
    body, err := json.Marshal(txtRecord)
    if err != nil {
        return err
    }

    _, err = c.doApiRequest(http.MethodPost, "record-txt", bytes.NewReader(body))
    return err
}

func (c *cloudDnsClient) DeleteRecord(zone, recordName string) (error) {
    domainId, err := c.getDomainId(zone)
    if err != nil {
        return err
    }

    recordId, err := c.getRecordId(domainId, recordName)
    if err != nil {
        return err
    }

    err = c.deleteRecordById(recordId)
    if err != nil {
        return err
    }

    err = c.publishRecords(domainId)
    return err
}

func (c *cloudDnsClient) deleteRecordById(recordId string) (error) {
    endpoint := fmt.Sprintf("record/%s", recordId)
    _, err := c.doApiRequest(http.MethodDelete, endpoint, nil)
    return err
}


// TODO Rewrite to use makeRequest
func (c *cloudDnsClient) doApiRequest(method, endpoint string, body io.Reader) ([]byte, error) {
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

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

	if resp.StatusCode >= 400 {
        fmt.Println(err)
		return nil, readError(req, resp)
	}

    content, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }

    return content, nil
}

func (c *cloudDnsClient) getDomainId(zone string) (string, error) {
    searchClient := searchBlock{Name: "clientId", Operator: "eq", Value: c.ClientId}
    searchDomain := searchBlock{Name: "domainName", Operator: "eq", Value: zone}
    searchBody := map[string][]searchBlock {"search": []searchBlock{searchClient, searchDomain}}

    body, err := json.Marshal(searchBody)
    if err != nil {
        return "", err
    }

    resp, err := c.doApiRequest(http.MethodPost, "domain/search", bytes.NewReader(body))
    if err != nil {
        return "", err
    }

    var result map[string]interface{}
    json.Unmarshal(resp, &result)

    // Let's dig for the .["items"][0]["id"] path 
    items := result["items"].([]interface{})
    domainDetails := items[0].(map[string]interface{})
    domainId := domainDetails["id"].(string)

    return domainId, nil
}

func (c *cloudDnsClient) getRecordId(domainId, recordName string) (string, error) {
    endpoint := fmt.Sprintf("domain/%s", domainId)
    resp, err := c.doApiRequest(http.MethodGet, endpoint, nil)
    if err != nil {
        return "", err
    }

    var result map[string]interface{}
    json.Unmarshal(resp, &result)

    recordId := ""
    entries := result["lastDomainRecordList"].([]interface{})
    for _, entry := range entries {
        entryMap := entry.(map[string]interface{})
        if entryMap["name"] == recordName && entryMap["type"] == "TXT" {
            recordId = entryMap["id"].(string)
        }
    }
    return recordId, nil
}

// TODO Rewrite to use makeRequest
func (c *cloudDnsClient) login() (error) {
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

	resp, err := c.HTTPClient.Do(req)
    fmt.Println(resp)
    fmt.Println(err)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return readError(req, resp)
	}

    body, err = ioutil.ReadAll(resp.Body)
    if err != nil {
        return err
    }

    var result map[string]interface{}
    json.Unmarshal([]byte(body), &result)

    authBlock := result["auth"].(map[string]interface{})
    c.AccessToken = authBlock["accessToken"].(string)

    return nil
}

func (c *cloudDnsClient) newRequest(method, reqURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))

	return req, nil
}

func (c *cloudDnsClient) publishRecords(domainId string) (error) {
    soaTtl := map[string]int {"soaTtl": c.TTL}
    body, err := json.Marshal(soaTtl)
    if err != nil {
        return err
    }

    endpoint := fmt.Sprintf("domain/%s/publish", domainId)
    _, err = c.doApiRequest(http.MethodPut, endpoint, bytes.NewReader(body))
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

//func main() {
//    email := "mail@radeksprta.eu"
//    password := "wahrNXz37pzOuteEvvLp"
//    clientId := "9bRkpqTNQ9m3_4SOvpkZQB"
//
//    client := NewCloudDnsClient(clientId, email, password)
//    client.login()
//
//    domainId, err := client.getDomainId("rodinnakniha.cz.")
//
//    //err = client.addTxtRecord(domainId, "lego.rodinnakniha.cz.", "test")
//    //if err != nil {
//    //    fmt.Println(err)
//    //}
//
//    //err = client.publishRecords(domainId)
//    //if err != nil {
//    //    fmt.Println(err)
//    //}
//
//    err = client.AddRecord("rodinnakniha.cz.", "lego.rodinnakniha.cz.", "test2")
//    if err != nil {
//        fmt.Println(err)
//    }
//
//    fmt.Println("Record added, sleeping")
//    time.Sleep(20 * time.Second)
//
//    recordId, err := client.getRecordId(domainId, "lego.rodinnakniha.cz.")
//    if err == nil {
//        fmt.Println(recordId)
//    }
//
//    //err = client.deleteRecordById(recordId)
//    //if err != nil {
//    //    fmt.Println(err)
//    //}
//
//    //err = client.publishRecords(domainId)
//    //if err != nil {
//    //    fmt.Println(err)
//    //}
//
//    //err = client.AddRecord("rodinnakniha.cz.", "lego.rodinnakniha.cz.", "test2")
//    //if err != nil {
//    //    fmt.Println(err)
//    //}
//
//    err = client.DeleteRecord("rodinnakniha.cz.", "lego.rodinnakniha.cz.")
//    if err != nil {
//        fmt.Println(err)
//    }
//}
