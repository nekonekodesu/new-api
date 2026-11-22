package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTryProtocolFallbackOnError_ClaudeToChatCompletionsWithToolsAndLearning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var messagesCalls atomic.Int32
	var chatCalls atomic.Int32
	var lastChatBody string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/messages":
			messagesCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"status_code=400, The request failed upstream. Please contact the administrator.","type":"new_api_error"}}`))
		case "/v1/chat/completions":
			chatCalls.Add(1)
			bodyBytes, _ := io.ReadAll(r.Body)
			lastChatBody = string(bodyBytes)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-fallback-1",
				"object":"chat.completion",
				"created":1700000000,
				"model":"deepseek-chat",
				"choices":[{
					"index":0,
					"message":{
						"role":"assistant",
						"content":"Checking weather now.",
						"tool_calls":[{
							"id":"call_weather_1",
							"type":"function",
							"function":{"name":"get_weather","arguments":"{\"city\":\"Taipei\"}"}
						}]
					},
					"finish_reason":"tool_calls"
				}],
				"usage":{"prompt_tokens":15,"completion_tokens":10,"total_tokens":25}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	claudeBody := `{
		"model":"deepseek-chat",
		"max_tokens":1024,
		"system":"You are a helpful assistant.",
		"messages":[{"role":"user","content":"What is the weather in Taipei?"}],
		"tools":[{
			"name":"get_weather",
			"description":"Get current weather",
			"input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}
		}]
	}`

	var claudeReq dto.ClaudeRequest
	require.NoError(t, common.Unmarshal([]byte(claudeBody), &claudeReq))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(claudeBody))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeUnknown,
		RelayFormat:     types.RelayFormatClaude,
		RequestURLPath:  "/v1/messages",
		OriginModelName: "deepseek-chat",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeNewAPI,
			ChannelId:            9901,
			ChannelBaseUrl:       upstream.URL,
			ApiType:              constant.APITypeNewAPI,
			ApiKey:               "test-key",
			UpstreamModelName:    "deepseek-chat",
			SupportStreamOptions: true,
		},
	}
	info.InitRequestConversionChain()

	adaptor := GetAdaptor(info.ApiType)
	require.NotNil(t, adaptor)
	adaptor.Init(info)

	converted, err := adaptor.ConvertClaudeRequest(c, info, &claudeReq)
	require.NoError(t, err)
	rawReq, err := common.Marshal(converted)
	require.NoError(t, err)
	bodyReader, closer, err := relaycommon.NewOutboundJSONBody(rawReq)
	require.NoError(t, err)
	defer closer.Close()

	respAny, err := adaptor.DoRequest(c, info, bodyReader)
	require.NoError(t, err)
	httpResp := respAny.(*http.Response)
	require.Equal(t, http.StatusBadRequest, httpResp.StatusCode)

	usage, fallbackErr, handled := TryProtocolFallbackOnError(c, info, adaptor, &claudeReq, httpResp, "")
	require.True(t, handled)
	require.Nil(t, fallbackErr)
	require.NotNil(t, usage)
	assert.Equal(t, 15, usage.PromptTokens)
	assert.Equal(t, 10, usage.CompletionTokens)
	assert.Equal(t, int32(1), messagesCalls.Load())
	assert.Equal(t, int32(1), chatCalls.Load())

	// Verify tool definition was converted to OpenAI format on the fallback request.
	assert.Equal(t, "get_weather", gjson.Get(lastChatBody, "tools.0.function.name").String())
	assert.Equal(t, "system", gjson.Get(lastChatBody, "messages.0.role").String())

	// Verify response sent back to client is valid Claude Messages format with tool_use.
	clientResp := rec.Body.String()
	assert.Equal(t, "message", gjson.Get(clientResp, "type").String())
	assert.Equal(t, "tool_use", gjson.Get(clientResp, "stop_reason").String())
	assert.Equal(t, "tool_use", gjson.Get(clientResp, "content.1.type").String())
	assert.Equal(t, "get_weather", gjson.Get(clientResp, "content.1.name").String())
	assert.Equal(t, "Taipei", gjson.Get(clientResp, "content.1.input.city").String())

	// Verify learned protocol format is applied on the next request so /v1/messages is skipped.
	nextInfo := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeUnknown,
		RelayFormat:     types.RelayFormatClaude,
		RequestURLPath:  "/v1/messages",
		OriginModelName: "deepseek-chat",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeNewAPI,
			ChannelId:         9901,
			ChannelBaseUrl:    upstream.URL,
			ApiType:           constant.APITypeNewAPI,
			UpstreamModelName: "deepseek-chat",
		},
	}
	ApplyLearnedProtocolFormat(nextInfo)
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAI), nextInfo.ForceTargetRelayFormat)
	assert.False(t, nextInfo.ShouldPassThroughBody())

	nextURL, err := adaptor.GetRequestURL(nextInfo)
	require.NoError(t, err)
	assert.Equal(t, upstream.URL+"/v1/chat/completions", nextURL)
}

func TestNewAPIAdaptorCrossProtocolRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Gemini client format to Claude model converts to /v1/messages", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &relaycommon.RelayInfo{
			RelayMode:       relayconstant.RelayModeGemini,
			RelayFormat:     types.RelayFormatGemini,
			RequestURLPath:  "/v1beta/models/claude-sonnet-4-5:generateContent",
			OriginModelName: "claude-sonnet-4-5",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:       constant.ChannelTypeNewAPI,
				ChannelBaseUrl:    "https://upstream.example",
				ApiType:           constant.APITypeNewAPI,
				UpstreamModelName: "claude-sonnet-4-5",
			},
		}
		adaptor := GetAdaptor(constant.APITypeNewAPI)
		adaptor.Init(info)

		geminiReq := &dto.GeminiChatRequest{
			Contents: []dto.GeminiChatContent{
				{Role: "user", Parts: []dto.GeminiPart{{Text: "Hello Claude from Gemini format"}}},
			},
		}
		converted, err := adaptor.ConvertGeminiRequest(c, info, geminiReq)
		require.NoError(t, err)
		assert.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())

		claudeOut, ok := converted.(*dto.ClaudeRequest)
		require.True(t, ok)
		assert.Equal(t, "claude-sonnet-4-5", claudeOut.Model)

		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://upstream.example/v1/messages", url)
	})

	t.Run("Gemini client format to DeepSeek model converts to /v1/chat/completions", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &relaycommon.RelayInfo{
			RelayMode:       relayconstant.RelayModeGemini,
			RelayFormat:     types.RelayFormatGemini,
			RequestURLPath:  "/v1beta/models/deepseek-chat:generateContent",
			OriginModelName: "deepseek-chat",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:       constant.ChannelTypeNewAPI,
				ChannelBaseUrl:    "https://upstream.example",
				ApiType:           constant.APITypeNewAPI,
				UpstreamModelName: "deepseek-chat",
			},
		}
		adaptor := GetAdaptor(constant.APITypeNewAPI)
		adaptor.Init(info)

		geminiReq := &dto.GeminiChatRequest{
			Contents: []dto.GeminiChatContent{
				{Role: "user", Parts: []dto.GeminiPart{{Text: "Hello DeepSeek from Gemini format"}}},
			},
		}
		converted, err := adaptor.ConvertGeminiRequest(c, info, geminiReq)
		require.NoError(t, err)
		assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAI), info.GetFinalRequestRelayFormat())

		oaiOut, ok := converted.(*dto.GeneralOpenAIRequest)
		require.True(t, ok)
		assert.Equal(t, "deepseek-chat", oaiOut.Model)

		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://upstream.example/v1/chat/completions", url)
	})

	t.Run("Claude client format to Gemini model converts to /v1beta/models/:model:generateContent", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &relaycommon.RelayInfo{
			RelayMode:       relayconstant.RelayModeUnknown,
			RelayFormat:     types.RelayFormatClaude,
			RequestURLPath:  "/v1/messages",
			OriginModelName: "gemini-2.5-pro",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:       constant.ChannelTypeNewAPI,
				ChannelBaseUrl:    "https://upstream.example",
				ApiType:           constant.APITypeNewAPI,
				UpstreamModelName: "gemini-2.5-pro",
			},
		}
		adaptor := GetAdaptor(constant.APITypeNewAPI)
		adaptor.Init(info)

		maxTokens := uint(1024)
		claudeReq := &dto.ClaudeRequest{
			Model:     "gemini-2.5-pro",
			MaxTokens: &maxTokens,
			Messages:  []dto.ClaudeMessage{{Role: "user", Content: "Hello Gemini from Claude format"}},
		}
		converted, err := adaptor.ConvertClaudeRequest(c, info, claudeReq)
		require.NoError(t, err)
		assert.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())

		_, ok := converted.(*dto.GeminiChatRequest)
		require.True(t, ok)

		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://upstream.example/v1beta/models/gemini-2.5-pro:generateContent", url)
	})
}
