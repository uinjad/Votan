package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Votan/internal/engine"
)

var (
	initialDataRegex = regexp.MustCompile(`(?:window\["ytInitialData"\]|var ytInitialData|ytInitialData)\s*=\s*({.+?});`)
	apiKeyRegex      = regexp.MustCompile(`"INNERTUBE_API_KEY"\s*:\s*"([^"]+)"`)
	clientVersion    = regexp.MustCompile(`"clientVersion"\s*:\s*"([^"]+)"`)
)

const sessionTTL = 20 * time.Minute

// ListenChat wraps the chat scraper in a rotation loop. It returns as soon as
// ctx is cancelled, so the caller can shut down cleanly.
func ListenChat(ctx context.Context, videoID string, commands chan<- engine.Command) {
	for {
		if ctx.Err() != nil {
			return
		}
		slog.Info("youtube: starting chat session", "video", videoID)

		sessionCtx, cancel := context.WithTimeout(ctx, sessionTTL)
		scrapeChatSession(sessionCtx, videoID, commands)
		cancel()

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			slog.Info("youtube: session rotated, reconnecting")
		}
	}
}

func scrapeChatSession(ctx context.Context, videoID string, commands chan<- engine.Command) {
	chatURL := "https://www.youtube.com/live_chat?v=" + videoID
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chatURL, nil)
	if err != nil {
		slog.Error("youtube: build request", "err", err)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", "CONSENT=YES+cb.20230509-09-p0.en+FX+999;")

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("youtube: connection error", "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("youtube: read body", "err", err)
		return
	}
	bodyStr := string(bodyBytes)

	apiKeyMatches := apiKeyRegex.FindStringSubmatch(bodyStr)
	clientVerMatches := clientVersion.FindStringSubmatch(bodyStr)
	initialDataMatches := initialDataRegex.FindStringSubmatch(bodyStr)

	if len(apiKeyMatches) < 2 || len(initialDataMatches) < 2 {
		slog.Warn("youtube: ytInitialData not found (stream offline or restricted)")
		return
	}

	apiKey := apiKeyMatches[1]
	clientVer := "2.20230509.00.00"
	if len(clientVerMatches) >= 2 {
		clientVer = clientVerMatches[1]
	}

	var ytData map[string]interface{}
	if err := json.Unmarshal([]byte(initialDataMatches[1]), &ytData); err != nil {
		slog.Warn("youtube: parse ytInitialData", "err", err)
		return
	}

	continuationToken := extractLiveChatToken(ytData)
	if continuationToken == "" {
		slog.Warn("youtube: chat token not found (chat disabled?)")
		return
	}
	slog.Info("youtube: connected, waiting for messages")

	// Remember session start so we can skip the chat history backlog.
	sessionStart := time.Now().UnixMicro()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}

		payload := fmt.Sprintf(`{"context":{"client":{"clientName":"WEB","clientVersion":"%s"}},"continuation":"%s"}`,
			clientVer, continuationToken)

		postReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://www.youtube.com/youtubei/v1/live_chat/get_live_chat?key="+apiKey,
			strings.NewReader(payload))
		if err != nil {
			continue
		}
		postReq.Header.Set("Content-Type", "application/json")
		postReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
		postReq.Header.Set("Cookie", "CONSENT=YES+cb.20230509-09-p0.en+FX+999;")

		postResp, err := client.Do(postReq)
		if err != nil {
			continue
		}

		var chatData map[string]interface{}
		decodeErr := json.NewDecoder(postResp.Body).Decode(&chatData)
		_ = postResp.Body.Close()
		if decodeErr != nil {
			slog.Debug("youtube: decode chat payload", "err", decodeErr)
			continue
		}

		newContinuation := updateContinuation(chatData, continuationToken)
		if newContinuation != continuationToken {
			continuationToken = newContinuation
			parseAndSendMessages(ctx, chatData, commands, sessionStart)
		} else {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func findKey(obj interface{}, key string) interface{} {
	switch val := obj.(type) {
	case map[string]interface{}:
		if v, ok := val[key]; ok {
			return v
		}
		for _, v := range val {
			if res := findKey(v, key); res != nil {
				return res
			}
		}
	case []interface{}:
		for _, v := range val {
			if res := findKey(v, key); res != nil {
				return res
			}
		}
	}
	return nil
}

func extractLiveChatToken(ytData map[string]interface{}) string {
	if itemsObj := findKey(ytData, "subMenuItems"); itemsObj != nil {
		if items, ok := itemsObj.([]interface{}); ok && len(items) > 0 {
			targetIndex := 0
			if len(items) > 1 {
				targetIndex = 1
			}
			if itemMap, ok := items[targetIndex].(map[string]interface{}); ok {
				if cont, ok := itemMap["continuation"].(map[string]interface{}); ok {
					if cmd, ok := cont["continuationCommand"].(map[string]interface{}); ok {
						if token, ok := cmd["token"].(string); ok {
							return token
						}
					}
				}
			}
		}
	}
	if rcdObj := findKey(ytData, "reloadContinuationData"); rcdObj != nil {
		if rcdMap, ok := rcdObj.(map[string]interface{}); ok {
			if token, ok := rcdMap["continuation"].(string); ok {
				return token
			}
		}
	}
	return ""
}

func updateContinuation(data map[string]interface{}, fallback string) string {
	if timed := findKey(data, "timedContinuationData"); timed != nil {
		if tMap, ok := timed.(map[string]interface{}); ok {
			if cont, ok := tMap["continuation"].(string); ok {
				return cont
			}
		}
	}
	if inval := findKey(data, "invalidationContinuationData"); inval != nil {
		if iMap, ok := inval.(map[string]interface{}); ok {
			if cont, ok := iMap["continuation"].(string); ok {
				return cont
			}
		}
	}
	return fallback
}

func parseAndSendMessages(ctx context.Context, data map[string]interface{}, commands chan<- engine.Command, sessionStart int64) {
	actionsObj := findKey(data, "actions")
	if actionsObj == nil {
		return
	}
	actions, ok := actionsObj.([]interface{})
	if !ok {
		return
	}

	for _, action := range actions {
		act, ok := action.(map[string]interface{})
		if !ok {
			continue
		}
		addChatItem, ok := act["addChatItemAction"].(map[string]interface{})
		if !ok {
			continue
		}
		item, ok := addChatItem["item"].(map[string]interface{})
		if !ok {
			continue
		}
		textMsg, ok := item["liveChatTextMessageRenderer"].(map[string]interface{})
		if !ok {
			continue
		}

		// Skip messages from before the session started (chat history).
		if tsStr, ok := textMsg["timestampUsec"].(string); ok {
			if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil && ts < sessionStart {
				continue
			}
		}

		authorName := "Глядач"
		if anObj := findKey(textMsg, "authorName"); anObj != nil {
			if anMap, ok := anObj.(map[string]interface{}); ok {
				if simple, ok := anMap["simpleText"].(string); ok {
					authorName = simple
				}
			}
		}

		authorID, _ := textMsg["authorExternalChannelId"].(string)

		var fullText string
		if messageObj, ok := textMsg["message"].(map[string]interface{}); ok {
			if runs, ok := messageObj["runs"].([]interface{}); ok {
				for _, r := range runs {
					if runMap, ok := r.(map[string]interface{}); ok {
						if t, ok := runMap["text"].(string); ok {
							fullText += t
						}
					}
				}
			}
		}

		if fullText != "" && authorID != "" {
			select {
			case commands <- engine.Command{PlayerID: authorID, PlayerName: authorName, Action: fullText}:
			case <-ctx.Done():
				return
			}
		}
	}
}
