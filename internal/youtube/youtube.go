package youtube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"Votan/internal/engine"
)

var (
	apiKeyRegex        = regexp.MustCompile(`"INNERTUBE_API_KEY":"([^"]+)"`)
	clientVersionRegex = regexp.MustCompile(`"clientVersion":"([^"]+)"`)
	continuationRegex  = regexp.MustCompile(`"continuation":"([^"]+)"`)
)

// ListenChat тепер працює як скрапер (анонімний глядач)
func ListenChat(videoID, _ string, commandChan chan<- engine.Command) {
	fmt.Println("📡 [Скрапер] Підключаємось до трансляції як анонімний глядач...")

	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Отримуємо початкову HTML-сторінку живого чату
	req, _ := http.NewRequest("GET", "https://www.youtube.com/live_chat?v="+videoID, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// БАЙПАС ВІКНА ЗГОДИ (Cookie Consent)
	req.Header.Set("Cookie", "CONSENT=YES+cb.20230101-00-p0.en+FX+478;")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Помилка завантаження чату:", err)
		return
	}
	defer resp.Body.Close()

	htmlBytes, _ := io.ReadAll(resp.Body)
	htmlStr := string(htmlBytes)

	// 2. Витягуємо внутрішні ключі YouTube за допомогою регулярних виразів
	apiKeyMatches := apiKeyRegex.FindStringSubmatch(htmlStr)
	clientVerMatches := clientVersionRegex.FindStringSubmatch(htmlStr)
	continuationMatches := continuationRegex.FindStringSubmatch(htmlStr)

	if len(apiKeyMatches) < 2 || len(continuationMatches) < 2 {
		fmt.Println("❌ Не вдалося знайти чат. Стрім точно запущено в OBS?")
		return
	}

	apiKey := apiKeyMatches[1]
	clientVersion := "2.20231214.00.00" // Запасна версія клієнта
	if len(clientVerMatches) >= 2 {
		clientVersion = clientVerMatches[1]
	}
	continuation := continuationMatches[1]

	fmt.Println("✅ [Скрапер] Успішно підключено! Починаємо читати чат в обхід лімітів та блокувань.")

	for {
		// 3. Формуємо запит до прихованого API (youtubei)
		payload := map[string]interface{}{
			"context": map[string]interface{}{
				"client": map[string]string{
					"clientName":    "WEB",
					"clientVersion": clientVersion,
				},
			},
			"continuation": continuation,
		}

		payloadBytes, _ := json.Marshal(payload)

		apiURL := "https://www.youtube.com/youtubei/v1/live_chat/get_live_chat?key=" + apiKey
		apiReq, _ := http.NewRequest("POST", apiURL, bytes.NewBuffer(payloadBytes))
		apiReq.Header.Set("Content-Type", "application/json")
		apiReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

		apiResp, err := client.Do(apiReq)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		// Структура внутрішньої відповіді YouTube
		var chatData struct {
			ContinuationContents struct {
				LiveChatContinuation struct {
					Actions []struct {
						AddChatItemAction struct {
							Item struct {
								LiveChatTextMessageRenderer struct {
									Message struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"message"`
									AuthorName struct {
										SimpleText string `json:"simpleText"`
									} `json:"authorName"`
									AuthorExternalChannelId string `json:"authorExternalChannelId"`
								} `json:"liveChatTextMessageRenderer"`
							} `json:"item"`
						} `json:"addChatItemAction"`
					} `json:"actions"`
					Continuations []struct {
						TimedContinuationData struct {
							Continuation string `json:"continuation"`
							TimeoutMs    int    `json:"timeoutMs"`
						} `json:"timedContinuationData"`
						InvalidationContinuationData struct {
							Continuation string `json:"continuation"`
							TimeoutMs    int    `json:"timeoutMs"`
						} `json:"invalidationContinuationData"`
					} `json:"continuations"`
				} `json:"liveChatContinuation"`
			} `json:"continuationContents"`
		}

		json.NewDecoder(apiResp.Body).Decode(&chatData)
		apiResp.Body.Close()

		contents := chatData.ContinuationContents.LiveChatContinuation

		// 4. Парсимо та відправляємо повідомлення в ігровий рушій
		for _, action := range contents.Actions {
			renderer := action.AddChatItemAction.Item.LiveChatTextMessageRenderer
			if renderer.AuthorName.SimpleText == "" {
				continue
			}

			fullMessage := ""
			for _, run := range renderer.Message.Runs {
				fullMessage += run.Text
			}

			if fullMessage != "" {
				fmt.Printf("💬 [Скрапер] %s: %s\n", renderer.AuthorName.SimpleText, fullMessage)
				commandChan <- engine.Command{
					PlayerID:   renderer.AuthorExternalChannelId,
					PlayerName: renderer.AuthorName.SimpleText,
					Action:     fullMessage, // Якщо тут буде !r5 - гравець побіжить
				}
			}
		}

		// 5. Оновлюємо токен для наступного запиту
		timeoutMs := 3000
		if len(contents.Continuations) > 0 {
			contData := contents.Continuations[0]
			if contData.TimedContinuationData.Continuation != "" {
				continuation = contData.TimedContinuationData.Continuation
				timeoutMs = contData.TimedContinuationData.TimeoutMs
			} else if contData.InvalidationContinuationData.Continuation != "" {
				continuation = contData.InvalidationContinuationData.Continuation
				timeoutMs = contData.InvalidationContinuationData.TimeoutMs
			}
		}

		// Захист від надто частих запитів
		if timeoutMs < 2000 {
			timeoutMs = 2000
		}
		time.Sleep(time.Duration(timeoutMs) * time.Millisecond)
	}
}