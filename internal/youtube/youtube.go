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

// Виносимо структуру для зручності та додаємо ВСІ необхідні json-теги
type YTChatResponse struct {
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

// ListenChat працює як анонімний глядач, збираючи повідомлення
func ListenChat(videoID string, commandChan chan<- engine.Command) {
	fmt.Println("📡 [Скрапер] Підключаємось до трансляції як анонімний глядач...")

	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Отримуємо початкову HTML-сторінку
	req, _ := http.NewRequest("GET", "https://www.youtube.com/live_chat?v="+videoID, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", "CONSENT=YES+cb.20230101-00-p0.en+FX+478;")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ [YouTube] Помилка завантаження чату:", err)
		return
	}
	defer resp.Body.Close()

	htmlBytes, _ := io.ReadAll(resp.Body)
	htmlStr := string(htmlBytes)

	// 2. Витягуємо ключі
	apiKeyMatches := apiKeyRegex.FindStringSubmatch(htmlStr)
	clientVerMatches := clientVersionRegex.FindStringSubmatch(htmlStr)
	continuationMatches := continuationRegex.FindStringSubmatch(htmlStr)

	if len(apiKeyMatches) < 2 || len(continuationMatches) < 2 {
		fmt.Println("❌ [YouTube] Не вдалося знайти чат. Стрім точно запущено або це прямий ефір?")
		return
	}

	apiKey := apiKeyMatches[1]
	clientVersion := "2.20231214.00.00"
	if len(clientVerMatches) >= 2 {
		clientVersion = clientVerMatches[1]
	}
	continuation := continuationMatches[1]

	fmt.Println("✅ [Скрапер] Успішно підключено! Починаємо читати чат.")

	for {
		// 3. Формуємо запит до youtubei
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
			fmt.Println("⚠️ [YouTube] Помилка з'єднання, повторюємо через 5с...")
			time.Sleep(5 * time.Second)
			continue
		}

		var chatData YTChatResponse
		err = json.NewDecoder(apiResp.Body).Decode(&chatData)
		apiResp.Body.Close()

		if err != nil {
			fmt.Println("⚠️ [YouTube] Помилка парсингу відповіді, можливо змінили API.")
			time.Sleep(5 * time.Second)
			continue
		}

		contents := chatData.ContinuationContents.LiveChatContinuation

		// 4. Парсимо повідомлення
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
				fmt.Printf("💬 [YouTube] %s: %s\n", renderer.AuthorName.SimpleText, fullMessage)
				commandChan <- engine.Command{
					PlayerID:   renderer.AuthorExternalChannelId,
					PlayerName: renderer.AuthorName.SimpleText,
					Action:     fullMessage,
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
		} else {
			// ВАЖЛИВО: Якщо масив Continuations порожній - чат завершився!
			fmt.Println("🛑 [YouTube] Чат закрито або стрім завершився.")
			break
		}

		// Захист від надто частих запитів (щоб YT не забанив IP)
		if timeoutMs < 2000 {
			timeoutMs = 2000
		}
		time.Sleep(time.Duration(timeoutMs) * time.Millisecond)
	}
}
