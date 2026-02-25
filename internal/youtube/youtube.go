package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

// ListenChat wraps the chat scraper in a 20-minute rotation loop.
func ListenChat(videoID string, commandChan chan<- engine.Command) {
	for {
		log.Printf("youtube: starting chat session for video %s (20m rotation)", videoID)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		scrapeChatSession(ctx, videoID, commandChan)
		cancel()

		log.Printf("youtube: session rotated. reconnecting in 5s...")
		time.Sleep(5 * time.Second)
	}
}

func scrapeChatSession(ctx context.Context, videoID string, commandChan chan<- engine.Command) {
	chatURL := "https://www.youtube.com/live_chat?v=" + videoID

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", chatURL, nil)
	if err != nil {
		log.Printf("youtube: failed to create request: %v", err)
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", "CONSENT=YES+cb.20230509-09-p0.en+FX+999;")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("youtube: connection error: %v", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	apiKeyMatches := apiKeyRegex.FindStringSubmatch(bodyStr)
	clientVerMatches := clientVersion.FindStringSubmatch(bodyStr)
	initialDataMatches := initialDataRegex.FindStringSubmatch(bodyStr)

	if len(apiKeyMatches) < 2 || len(initialDataMatches) < 2 {
		log.Printf("youtube: failed to find ytInitialData. Stream might be offline or restricted.")
		return
	}

	apiKey := apiKeyMatches[1]
	clientVer := "2.20230509.00.00"
	if len(clientVerMatches) >= 2 {
		clientVer = clientVerMatches[1]
	}

	var ytData map[string]interface{}
	err = json.Unmarshal([]byte(initialDataMatches[1]), &ytData)
	if err != nil {
		log.Printf("youtube: json parse error: %v", err)
		return
	}

	continuationToken := extractLiveChatToken(ytData)
	if continuationToken == "" {
		log.Printf("youtube: chat token not found. Chat might be disabled.")
		return
	}

	log.Println("youtube: connected successfully, waiting for messages...")

	// ЗАПАМ'ЯТОВУЄМО ЧАС СТАРТУ СЕСІЇ (щоб відсіяти старі повідомлення)
	sessionStartTime := time.Now().UnixMicro()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}

		payload := fmt.Sprintf(`{
            "context": {"client": {"clientName": "WEB", "clientVersion": "%s"}},
            "continuation": "%s"
        }`, clientVer, continuationToken)

		postReq, err := http.NewRequestWithContext(ctx, "POST", "https://www.youtube.com/youtubei/v1/live_chat/get_live_chat?key="+apiKey, strings.NewReader(payload))
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
		json.NewDecoder(postResp.Body).Decode(&chatData)
		postResp.Body.Close()

		newContinuation := updateContinuation(chatData, continuationToken)
		if newContinuation != continuationToken {
			continuationToken = newContinuation
			// Передаємо час старту сесії для фільтрації
			parseAndSendMessages(chatData, commandChan, sessionStartTime)
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
		if items, ok := itemsObj.([]interface{}); ok {
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

func parseAndSendMessages(data map[string]interface{}, commandChan chan<- engine.Command, sessionStartTime int64) {
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

		if addChatItem, ok := act["addChatItemAction"].(map[string]interface{}); ok {
			item, ok := addChatItem["item"].(map[string]interface{})
			if !ok {
				continue
			}

			if textMsg, ok := item["liveChatTextMessageRenderer"].(map[string]interface{}); ok {

				// ФІЛЬТРАЦІЯ СТАРИХ ПОВІДОМЛЕНЬ (з історії)
				if tsStr, ok := textMsg["timestampUsec"].(string); ok {
					if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
						if ts < sessionStartTime {
							continue // Ігноруємо все, що було до старту скрапера
						}
					}
				}

				authorName := "Глядач"
				if anObj := findKey(textMsg, "authorName"); anObj != nil {
					if simple, ok := anObj.(map[string]interface{})["simpleText"].(string); ok {
						authorName = simple
					}
				}

				authorId := ""
				if idObj, ok := textMsg["authorExternalChannelId"].(string); ok {
					authorId = idObj
				}

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

				if fullText != "" && authorId != "" {
					commandChan <- engine.Command{
						PlayerID:   authorId,
						PlayerName: authorName,
						Action:     fullText,
					}
				}
			}
		}
	}
}
