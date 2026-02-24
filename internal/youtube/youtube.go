package youtube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"Votan/internal/engine"
)

var (
	initialDataRegex = regexp.MustCompile(`(?:window\["ytInitialData"\]|var ytInitialData|ytInitialData)\s*=\s*({.+?});`)
	apiKeyRegex      = regexp.MustCompile(`"INNERTUBE_API_KEY"\s*:\s*"([^"]+)"`)
	clientVersion    = regexp.MustCompile(`"clientVersion"\s*:\s*"([^"]+)"`)
)

// РЕКУРСИВНИЙ ПОШУК КЛЮЧА В JSON
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

func ListenChat(videoID string, commandChan chan<- engine.Command) {
	chatURL := "https://www.youtube.com/live_chat?v=" + videoID
	fmt.Printf("📺 Запуск парсера YouTube... (Слухаємо: %s)\n", chatURL)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", chatURL, nil)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", "CONSENT=YES+cb.20230509-09-p0.en+FX+999;")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Помилка підключення до YouTube:", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	apiKeyMatches := apiKeyRegex.FindStringSubmatch(bodyStr)
	clientVerMatches := clientVersion.FindStringSubmatch(bodyStr)
	initialDataMatches := initialDataRegex.FindStringSubmatch(bodyStr)

	if len(apiKeyMatches) < 2 || len(initialDataMatches) < 2 {
		fmt.Println("❌ Не вдалося знайти ytInitialData. Стрім недоступний або має обмеження.")
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
		fmt.Println("❌ Помилка парсингу JSON від YouTube:", err)
		return
	}

	continuationToken := extractLiveChatToken(ytData)
	if continuationToken == "" {
		fmt.Println("❌ Токен чату не знайдено. Схоже, це не прямий ефір або чат вимкнено.")
		return
	}

	fmt.Println("✅ YouTube Chat підключено (Режим: Всі повідомлення)! Очікування...")

	for {
		time.Sleep(1500 * time.Millisecond)

		payload := fmt.Sprintf(`{
			"context": {"client": {"clientName": "WEB", "clientVersion": "%s"}},
			"continuation": "%s"
		}`, clientVer, continuationToken)

		postReq, _ := http.NewRequest("POST", "https://www.youtube.com/youtubei/v1/live_chat/get_live_chat?key="+apiKey, strings.NewReader(payload))
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
			parseAndSendMessages(chatData, commandChan)
		} else {
			time.Sleep(2 * time.Second)
		}
	}
}

func extractLiveChatToken(ytData map[string]interface{}) string {
	// 1. Шукаємо меню перемикання чату
	if itemsObj := findKey(ytData, "subMenuItems"); itemsObj != nil {
		if items, ok := itemsObj.([]interface{}); ok {
			// Індекс 0 = Top Chat (Цікаві), Індекс 1 = Live Chat (Всі повідомлення)
			targetIndex := 0
			if len(items) > 1 {
				targetIndex = 1 // Жорстко обираємо друге меню, ігноруючи текст (локалізацію)
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

	// 2. Якщо меню немає, шукаємо просто перший ліпший токен оновлення
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

func parseAndSendMessages(data map[string]interface{}, commandChan chan<- engine.Command) {
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
