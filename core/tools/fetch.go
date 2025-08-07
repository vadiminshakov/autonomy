package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

)

func init() {
	Register("fetch", fetchURL)
}

// fetchURL fetches content from a URL and returns it in the specified format
func fetchURL(args map[string]interface{}) (string, error) {
	// get URL parameter
	urlVal, ok := args["url"].(string)
	if !ok || strings.TrimSpace(urlVal) == "" {
		return "", fmt.Errorf("parameter 'url' must be a non-empty string")
	}

	// get format parameter
	formatVal, ok := args["format"].(string)
	if !ok || strings.TrimSpace(formatVal) == "" {
		formatVal = "text" // default format
	}

	format := strings.ToLower(strings.TrimSpace(formatVal))
	if format != "text" && format != "markdown" && format != "html" {
		return "", fmt.Errorf("format must be one of: text, markdown, html")
	}

	// validate URL
	if !strings.HasPrefix(urlVal, "http://") && !strings.HasPrefix(urlVal, "https://") {
		return "", fmt.Errorf("URL must start with http:// or https://")
	}

	// get optional timeout parameter
	timeoutVal := 30 // default 30 seconds
	if timeoutInterface, ok := args["timeout"]; ok {
		if timeoutFloat, ok := timeoutInterface.(float64); ok {
			timeoutVal = int(timeoutFloat)
		}
	}

	// limit timeout to reasonable bounds
	if timeoutVal <= 0 || timeoutVal > 120 {
		timeoutVal = 30
	}

	// create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(timeoutVal) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// create request
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutVal)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", urlVal, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "autonomy/1.0")

	// make request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	// read response body with size limit (5MB)
	maxSize := int64(5 * 1024 * 1024)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	content := string(body)

	// validate UTF-8
	if !utf8.ValidString(content) {
		return "", fmt.Errorf("response content is not valid UTF-8")
	}

	contentType := resp.Header.Get("Content-Type")

	// process content based on format
	switch format {
	case "text":
		if strings.Contains(contentType, "text/html") {
			text, err := extractTextFromHTML(content)
			if err != nil {
				return "", fmt.Errorf("failed to extract text from HTML: %v", err)
			}
			content = text
		}

	case "markdown":
		if strings.Contains(contentType, "text/html") {
			// basic HTML to markdown conversion - extract main content
			content = basicHTMLToMarkdown(content)
		}

	case "html":
		// return HTML as-is for html format
		if strings.Contains(contentType, "text/html") {
			// basic body extraction without external dependencies
			content = extractHTMLBody(content)
		}
	}

	// truncate if too large
	const maxContentSize = 250 * 1024 // 250KB
	if len(content) > maxContentSize {
		content = content[:maxContentSize]
		content += fmt.Sprintf("\n\n[Content truncated to %d bytes]", maxContentSize)
	}

	// record operation
	state := getTaskState()
	state.mu.Lock()
	if state.CompletedTools == nil {
		state.CompletedTools = make(map[string]int)
	}
	state.CompletedTools["fetch"]++
	state.mu.Unlock()

	// create structured response
	metadata := &FetchMetadata{
		URL:         urlVal,
		Format:      format,
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Size:        len(content),
	}

	return CreateStructuredResponse(fmt.Sprintf("Fetched content from %s in %s format", urlVal, format), metadata), nil
}

// extractTextFromHTML extracts plain text from HTML content using basic parsing
func extractTextFromHTML(html string) (string, error) {
	// basic text extraction - remove HTML tags
	text := html
	text = strings.ReplaceAll(text, "<script", "<!--<script")
	text = strings.ReplaceAll(text, "</script>", "</script>-->")
	text = strings.ReplaceAll(text, "<style", "<!--<style")
	text = strings.ReplaceAll(text, "</style>", "</style>-->")
	
	// remove HTML tags with simple regex-like approach
	var result strings.Builder
	inTag := false
	for _, char := range text {
		if char == '<' {
			inTag = true
		} else if char == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(char)
		}
	}
	
	text = result.String()
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.Join(strings.Fields(text), " ")
	
	return text, nil
}

// basicHTMLToMarkdown converts HTML to basic markdown format
func basicHTMLToMarkdown(html string) string {
	// basic HTML to markdown conversion
	text := html
	text = strings.ReplaceAll(text, "<h1", "\n# ")
	text = strings.ReplaceAll(text, "<h2", "\n## ")
	text = strings.ReplaceAll(text, "<h3", "\n### ")
	text = strings.ReplaceAll(text, "<p>", "\n\n")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<strong>", "**")
	text = strings.ReplaceAll(text, "</strong>", "**")
	text = strings.ReplaceAll(text, "<em>", "*")
	text = strings.ReplaceAll(text, "</em>", "*")
	
	// remove remaining HTML tags
	var result strings.Builder
	inTag := false
	for _, char := range text {
		if char == '<' {
			inTag = true
		} else if char == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(char)
		}
	}
	
	return result.String()
}

// extractHTMLBody extracts body content from HTML
func extractHTMLBody(html string) string {
	// find body content using simple string operations
	bodyStart := strings.Index(strings.ToLower(html), "<body")
	if bodyStart == -1 {
		return html // return as-is if no body tag
	}
	
	// find the end of opening body tag
	bodyOpenEnd := strings.Index(html[bodyStart:], ">")
	if bodyOpenEnd == -1 {
		return html
	}
	bodyOpenEnd += bodyStart + 1
	
	// find closing body tag
	bodyEnd := strings.Index(strings.ToLower(html), "</body>")
	if bodyEnd == -1 {
		return html[bodyOpenEnd:] // return from body opening to end
	}
	
	return "<html>\n<body>\n" + html[bodyOpenEnd:bodyEnd] + "\n</body>\n</html>"
}

// FetchMetadata contains metadata for fetch operations
type FetchMetadata struct {
	URL         string `json:"url"`
	Format      string `json:"format"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}