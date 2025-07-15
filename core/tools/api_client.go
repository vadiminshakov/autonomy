package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func init() {
	Register("post_request", PostRequest)
	Register("get_request", GetRequest)
}

// PostRequest makes a POST request to the specified endpoint with the provided body and headers
func PostRequest(args map[string]interface{}) (string, error) {
	response, err := makeRequest("POST", args)
	if err != nil {
		return "", err
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("error marshalling response to JSON: %v", err)
	}

	return string(responseJSON), nil
}

// GetRequest makes a GET request to the specified endpoint with the provided headers
func GetRequest(args map[string]interface{}) (string, error) {
	response, err := makeRequest("GET", args)
	if err != nil {
		return "", err
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("error marshalling response to JSON: %v", err)
	}

	return string(responseJSON), nil
}

func makeRequest(method string, args map[string]interface{}) (map[string]interface{}, error) {
	baseURL, ok := args["baseURL"].(string)
	if !ok || baseURL == "" {
		return nil, errors.New("baseURL must be a non-empty string")
	}

	endpoint, ok := args["endpoint"].(string)
	if !ok || endpoint == "" {
		return nil, errors.New("endpoint must be a non-empty string")
	}

	apiKey, _ := args["apiKey"].(string)
	userAgent, _ := args["userAgent"].(string)

	headers, _ := args["headers"].(map[string]string)
	if headers == nil {
		headers = make(map[string]string)
	}

	var reqBody io.Reader
	if method == "POST" {
		body, ok := args["body"]
		if !ok {
			return nil, errors.New("body must be provided for POST request")
		}

		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("error marshalling request body: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	var responseData map[string]interface{}
	if err := json.Unmarshal(respBody, &responseData); err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}

	if errorData, exists := responseData["error"]; exists {
		return nil, fmt.Errorf("API error: %v", errorData)
	}

	return responseData, nil
}
