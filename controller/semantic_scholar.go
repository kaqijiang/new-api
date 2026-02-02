package controller

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const SemanticScholarModelName = "semantic-scholar"

// SemanticScholarProxy handles passthrough requests to Semantic Scholar API
func SemanticScholarProxy(c *gin.Context) {
	startTime := time.Now()

	// Get the path after /s2/
	path := c.Param("path")
	apiType := c.GetString("api") // graph, recommendations, or datasets (set by router)

	// Build the full path
	fullPath := fmt.Sprintf("/%s%s", apiType, path)
	if c.Request.URL.RawQuery != "" {
		fullPath += "?" + c.Request.URL.RawQuery
	}

	// Get the token from context (set by TokenAuth middleware)
	tokenId := c.GetInt(string(constant.ContextKeyTokenId))
	userId := c.GetInt(string(constant.ContextKeyUserId))
	tokenKey := c.GetString(string(constant.ContextKeyTokenKey))
	tokenUnlimited := c.GetBool(string(constant.ContextKeyTokenUnlimited))
	tokenName := c.GetString("token_name")
	group := c.GetString(string(constant.ContextKeyUsingGroup))

	if tokenId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	// Get model price and calculate quota
	modelPrice, hasPrice := ratio_setting.GetModelPrice(SemanticScholarModelName, false)
	if !hasPrice {
		// Use default price if not configured (0.001 per call)
		modelPrice = 0.1
	}
	groupRatio := ratio_setting.GetGroupRatio(group)
	quota := int(modelPrice * common.QuotaPerUnit * groupRatio)

	// Check user quota
	userQuota, err := model.GetUserQuota(userId, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get user quota",
		})
		return
	}
	if userQuota < quota {
		c.JSON(http.StatusForbidden, gin.H{
			"error": fmt.Sprintf("用户额度不足，需要 %d，剩余 %d", quota, userQuota),
		})
		return
	}

	// Check token quota if not unlimited
	if !tokenUnlimited {
		token, err := model.GetTokenByKey(tokenKey, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to get token",
			})
			return
		}
		if token.RemainQuota < quota {
			c.JSON(http.StatusForbidden, gin.H{
				"error": fmt.Sprintf("令牌额度不足，需要 %d，剩余 %d", quota, token.RemainQuota),
			})
			return
		}
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

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to read upstream response",
		})
		return
	}

	// Calculate use time
	useTimeSeconds := int(time.Since(startTime).Seconds())

	// Only charge if upstream request was successful (2xx status code)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && quota > 0 {
		// Deduct user quota
		err = model.DecreaseUserQuota(userId, quota)
		if err != nil {
			// Log error but don't fail the request
			common.SysError(fmt.Sprintf("failed to decrease user quota: %v", err))
		}

		// Deduct token quota if not unlimited
		if !tokenUnlimited {
			err = model.DecreaseTokenQuota(tokenId, tokenKey, quota)
			if err != nil {
				common.SysError(fmt.Sprintf("failed to decrease token quota: %v", err))
			}
		}

		// Update statistics
		model.UpdateUserUsedQuotaAndRequestCount(userId, quota)
		model.UpdateChannelUsedQuota(channel.Id, quota)
	} else if resp.StatusCode >= 400 {
		// Request failed, don't charge
		quota = 0
	}

	// Log the request
	logContent := fmt.Sprintf("S2 API: %s %s, 状态码: %d, 模型价格: %.4f, 分组倍率: %.2f",
		c.Request.Method, fullPath, resp.StatusCode, modelPrice, groupRatio)

	// Build other info for log display (similar to MJ/Task)
	other := map[string]interface{}{
		"model_price":      modelPrice,
		"group_ratio":      groupRatio,
		"completion_ratio": 0.0,
		"model_ratio":      0.0,
		"per_call":         true,
		"request_path":     c.Request.URL.Path,
	}

	model.RecordConsumeLog(c, userId, model.RecordConsumeLogParams{
		ChannelId:        channel.Id,
		PromptTokens:     0,
		CompletionTokens: 0,
		ModelName:        SemanticScholarModelName,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          tokenId,
		UseTimeSeconds:   useTimeSeconds,
		IsStream:         false,
		Group:            group,
		Other:            other,
	})

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}
