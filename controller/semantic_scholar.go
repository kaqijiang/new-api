package controller

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// SemanticScholarProxy handles passthrough requests to Semantic Scholar API
func SemanticScholarProxy(c *gin.Context) {
	// Get the path after /s2/
	path := c.Param("path")
	apiType := c.GetString("api") // graph, recommendations, or datasets (set by router)

	// Build the full path
	fullPath := fmt.Sprintf("/%s/v1%s", apiType, path)
	if c.Request.URL.RawQuery != "" {
		fullPath += "?" + c.Request.URL.RawQuery
	}

	// Get the token from context (set by TokenAuth middleware)
	tokenId := c.GetInt(string(constant.ContextKeyTokenId))
	if tokenId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	// Get a Semantic Scholar channel by type
	channel, err := model.GetEnabledChannelByType(constant.ChannelTypeSemanticScholar)
	if err != nil || channel == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "no available semantic scholar channel",
		})
		return
	}

	// Get the API key (supports multi-key rotation)
	apiKey, _, keyErr := channel.GetNextEnabledKey()
	if keyErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "no available api key",
		})
		return
	}

	// Build the upstream URL
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.semanticscholar.org"
	}
	upstreamURL := baseURL + fullPath

	// Create the upstream request
	var bodyReader io.Reader
	if c.Request.Body != nil {
		bodyReader = c.Request.Body
	}

	req, err := http.NewRequest(c.Request.Method, upstreamURL, bodyReader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create request",
		})
		return
	}

	// Copy relevant headers from the original request
	for key, values := range c.Request.Header {
		// Skip authorization headers and host
		lowerKey := strings.ToLower(key)
		if lowerKey == "authorization" || lowerKey == "host" || lowerKey == "x-api-key" {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Set the Semantic Scholar API key
	req.Header.Set("x-api-key", apiKey)

	// Set content type if not set
	if req.Header.Get("Content-Type") == "" && c.Request.Method != "GET" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Get proxy from channel settings if configured
	channelSetting := channel.GetSetting()
	proxyURL := channelSetting.Proxy

	// Get HTTP client with proxy support
	client, err := service.GetHttpClientWithProxy(proxyURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to create http client: %v", err),
		})
		return
	}

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("upstream request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Read and write response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to read upstream response",
		})
		return
	}

	// Log the request for quota tracking (async)
	userId := c.GetInt(string(constant.ContextKeyUserId))
	tokenName := c.GetString("token_name")
	group := c.GetString(string(constant.ContextKeyUsingGroup))
	go func() {
		logContent := fmt.Sprintf("S2 API: %s %s", c.Request.Method, fullPath)
		model.RecordConsumeLog(c, userId, model.RecordConsumeLogParams{
			ChannelId:        channel.Id,
			PromptTokens:     0,
			CompletionTokens: 0,
			ModelName:        "semantic-scholar",
			TokenName:        tokenName,
			Quota:            0,
			Content:          logContent,
			TokenId:          tokenId,
			UseTimeSeconds:   0,
			IsStream:         false,
			Group:            group,
			Other:            nil,
		})
	}()

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}
