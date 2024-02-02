package controller

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channel/openai"
	"github.com/songquanpeng/one-api/relay/util"

	"github.com/gin-gonic/gin"
)

func testChannel(channel *model.Channel, request openai.ChatRequest) (err error, openaiErr *openai.Error) {
	switch channel.Type {
	case common.ChannelTypePaLM:
		fallthrough
	case common.ChannelTypeGemini:
		fallthrough
	case common.ChannelTypeAnthropic:
		fallthrough
	case common.ChannelTypeBaidu:
		fallthrough
	case common.ChannelTypeZhipu:
		fallthrough
	case common.ChannelTypeAli:
		fallthrough
	case common.ChannelType360:
		fallthrough
	case common.ChannelTypeXunfei:
		return errors.New("该渠道类型当前版本不支持测试，请手动测试"), nil
	case common.ChannelTypeAzure:
		request.Model = "gpt-35-turbo"
		defer func() {
			if err != nil {
				err = errors.New("请确保已在 Azure 上创建了 gpt-35-turbo 模型，并且 apiVersion 已正确填写！")
			}
		}()
	default:
		request.Model = "gpt-3.5-turbo"
	}
	requestURL := common.ChannelBaseURLs[channel.Type]
	if channel.Type == common.ChannelTypeAzure {
		requestURL = util.GetFullRequestURL(channel.GetBaseURL(), fmt.Sprintf("/openai/deployments/%s/chat/completions?api-version=2023-03-15-preview", request.Model), channel.Type)
	} else {
		if baseURL := channel.GetBaseURL(); len(baseURL) > 0 {
			requestURL = baseURL
		}

		requestURL = util.GetFullRequestURL(requestURL, "/v1/chat/completions", channel.Type)
	}
	jsonData, err := json.Marshal(request)
	if err != nil {
		return err, nil
	}
	req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err, nil
	}
	if channel.Type == common.ChannelTypeAzure {
		req.Header.Set("api-key", channel.Key)
	} else {
		req.Header.Set("Authorization", "Bearer "+channel.Key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := util.HTTPClient.Do(req)
	if err != nil {
		return err, nil
	}
	defer resp.Body.Close()
	if err != nil {
		return err, nil
	}

	if channel.AllowStreaming == common.ChannelAllowStreamEnabled {
		responseText := ""
		scanner := bufio.NewScanner(resp.Body)
		scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			if atEOF && len(data) == 0 {
				return 0, nil, nil
			}
			if i := strings.Index(string(data), "\n"); i >= 0 {
				return i + 1, data[0:i], nil
			}
			if atEOF {
				return len(data), data, nil
			}
			return 0, nil, nil
		})
		for scanner.Scan() {
			data := scanner.Text()
			if len(data) < 6 { // ignore blank line or wrong format
				continue
			}
			// ChatGPT Next Web
			if strings.HasPrefix(data, "event:") || strings.Contains(data, "event:") {
				// Remove event: event in the front or back
				data = strings.TrimPrefix(data, "event: event")
				data = strings.TrimSuffix(data, "event: event")
				// Remove everything, only keep `data: {...}` <--- this is the json
				// Find the start and end indices of `data: {...}` substring
				startIndex := strings.Index(data, "data:")
				endIndex := strings.LastIndex(data, "}")

				// If both indices are found and end index is greater than start index
				if startIndex != -1 && endIndex != -1 && endIndex > startIndex {
					// Extract the `data: {...}` substring
					data = data[startIndex : endIndex+1]
				}
			}
			if !strings.HasPrefix(data, "data:") {
				continue
			}
			data = strings.TrimPrefix(data, "data:")
			if !strings.HasPrefix(data, "[DONE]") {
				var streamResponse openai.ChatCompletionsStreamResponse
				err := json.Unmarshal([]byte(data), &streamResponse)
				if err == nil {
					for _, choice := range streamResponse.Choices {
						responseText += choice.Delta.Content
					}
				}
			}
		}

		if responseText == "" {
			return errors.New("Empty response"), nil
		}
	} else if channel.AllowNonStreaming == common.ChannelAllowNonStreamEnabled {
		var response openai.SlimTextResponse
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err, nil
		}
		err = json.Unmarshal(body, &response)
		if err != nil {
			return fmt.Errorf("Error: %s\nResp body: %s", err, body), nil
		}
		if response.Usage.CompletionTokens == 0 {
			if response.Error.Message == "" {
				response.Error.Message = "补全 tokens 非预期返回 0"
			}
			return fmt.Errorf("type %s, code %v, message %s", response.Error.Type, response.Error.Code, response.Error.Message), &response.Error
		}
	}

	return nil, nil
}

func buildTestRequest(stream bool) *openai.ChatRequest {
	testRequest := &openai.ChatRequest{
		Model:     "", // this will be set later
		MaxTokens: 1,
		Stream:    stream,
	}
	testMessage := openai.Message{
		Role:    "user",
		Content: "Say hi only",
	}
	testRequest.Messages = append(testRequest.Messages, testMessage)
	return testRequest
}

func TestChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	testRequest := buildTestRequest(channel.AllowStreaming == common.ChannelAllowStreamEnabled)
	tik := time.Now()
	err, _ = testChannel(channel, *testRequest)
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
			"time":    consumedTime,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}

var testAllChannelsLock sync.Mutex
var testAllChannelsRunning bool = false

func notifyRootUser(subject string, content string) {
	if config.RootUserEmail == "" {
		config.RootUserEmail = model.GetRootUserEmail()
	}
	err := common.SendEmail(subject, config.RootUserEmail, content)
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to send email: %s", err.Error()))
	}
}

// disable & notify
func disableChannel(channelId int, channelName string, reason string) {
	model.UpdateChannelStatusById(channelId, common.ChannelStatusAutoDisabled)
	subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelName, channelId)
	content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelName, channelId, reason)
	notifyRootUser(subject, content)
}

// enable & notify
func enableChannel(channelId int, channelName string) {
	model.UpdateChannelStatusById(channelId, common.ChannelStatusEnabled)
	subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
	content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
	notifyRootUser(subject, content)
}

func testAllChannels(notify bool) error {
	if config.RootUserEmail == "" {
		config.RootUserEmail = model.GetRootUserEmail()
	}
	testAllChannelsLock.Lock()
	if testAllChannelsRunning {
		testAllChannelsLock.Unlock()
		return errors.New("测试已在运行中")
	}
	testAllChannelsRunning = true
	testAllChannelsLock.Unlock()
	channels, err := model.GetAllChannels(0, 0, true)
	if err != nil {
		return err
	}
	var disableThreshold = int64(config.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // a impossible value
	}
	go func() {
		for _, channel := range channels {
			isChannelEnabled := channel.Status == common.ChannelStatusEnabled
			tik := time.Now()
			testRequest := buildTestRequest(channel.AllowStreaming == common.ChannelAllowStreamEnabled)
			err, openaiErr := testChannel(channel, *testRequest)
			tok := time.Now()
			milliseconds := tok.Sub(tik).Milliseconds()
			if isChannelEnabled && milliseconds > disableThreshold {
				err = fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
				disableChannel(channel.Id, channel.Name, err.Error())
			}
			if isChannelEnabled && util.ShouldDisableChannel(openaiErr, -1) {
				disableChannel(channel.Id, channel.Name, err.Error())
			}
			if !isChannelEnabled && util.ShouldEnableChannel(err, openaiErr) {
				enableChannel(channel.Id, channel.Name)
			}
			channel.UpdateResponseTime(milliseconds)
			time.Sleep(config.RequestInterval)
		}
		testAllChannelsLock.Lock()
		testAllChannelsRunning = false
		testAllChannelsLock.Unlock()
		if notify {
			err := common.SendEmail("通道测试完成", config.RootUserEmail, "通道测试完成，如果没有收到禁用通知，说明所有通道都正常")
			if err != nil {
				logger.SysError(fmt.Sprintf("failed to send email: %s", err.Error()))
			}
		}
	}()
	return nil
}

func TestAllChannels(c *gin.Context) {
	err := testAllChannels(true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func AutomaticallyTestChannels(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Minute)
		logger.SysLog("testing all channels")
		_ = testAllChannels(false)
		logger.SysLog("channel test finished")
	}
}
