package obs

import (
	"log"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/filters"
	"github.com/andreykaipov/goobs/api/requests/mediainputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
)

type Client struct {
	conn *goobs.Client

	// Додаємо захист від "гонитви анімацій"
	mu      sync.Mutex
	fadeGen int
}

func NewClient(addr, password string) (*Client, error) {
	c, err := goobs.New(addr, goobs.WithPassword(password))
	if err != nil {
		return nil, err
	}
	return &Client{conn: c}, nil
}

// nextFadeGen збільшує лічильник анімацій і повертає його
func (c *Client) nextFadeGen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fadeGen++
	return c.fadeGen
}

func (c *Client) SetSourceEnabled(sceneName, sourceName string, enabled bool) {
	idReq := &sceneitems.GetSceneItemIdParams{
		SceneName:  &sceneName,
		SourceName: &sourceName,
	}

	resp, err := c.conn.SceneItems.GetSceneItemId(idReq)
	if err != nil {
		log.Printf("OBS: Не знайдено джерело %s у сцені %s: %v", sourceName, sceneName, err)
		return
	}

	enableReq := &sceneitems.SetSceneItemEnabledParams{
		SceneItemEnabled: &enabled,
		SceneItemId:      &resp.SceneItemId,
		SceneName:        &sceneName,
	}

	_, err = c.conn.SceneItems.SetSceneItemEnabled(enableReq)
	if err != nil {
		log.Printf("OBS: Не вдалося змінити статус джерела %s: %v", sourceName, err)
	}
}

// FadeSourceOpacity тепер перевіряє, чи не скасували її
func (c *Client) FadeSourceOpacity(sourceName, filterName string, startOpacity, endOpacity float64, duration time.Duration) {
	// Фіксуємо "покоління" цієї конкретної анімації
	currentGen := c.nextFadeGen()

	steps := 20
	sleepTime := duration / time.Duration(steps)
	stepSize := (endOpacity - startOpacity) / float64(steps)
	current := startOpacity

	bTrue := true

	for i := 0; i <= steps; i++ {
		// Перевіряємо, чи не запустив хтось іншу анімацію або SetOpacity
		c.mu.Lock()
		if c.fadeGen != currentGen {
			c.mu.Unlock()
			// Якщо покоління змінилося — тихо вбиваємо цю горутину
			return
		}
		c.mu.Unlock()

		req := &filters.SetSourceFilterSettingsParams{
			SourceName: &sourceName,
			FilterName: &filterName,
			FilterSettings: map[string]interface{}{
				"opacity": current,
			},
			Overlay: &bTrue,
		}

		_, err := c.conn.Filters.SetSourceFilterSettings(req)
		if err != nil {
			log.Printf("OBS Fade Error: %v", err)
			return
		}

		current += stepSize
		time.Sleep(sleepTime)
	}
}

// SetOpacity миттєво встановлює прозорість і ВБИВАЄ всі старі плавні анімації
func (c *Client) SetOpacity(sourceName, filterName string, opacity float64) {
	// Збільшуємо лічильник, щоб усі старі FadeSourceOpacity одразу зупинилися
	c.nextFadeGen()

	bTrue := true
	req := &filters.SetSourceFilterSettingsParams{
		SourceName: &sourceName,
		FilterName: &filterName,
		FilterSettings: map[string]interface{}{
			"opacity": opacity,
		},
		Overlay: &bTrue,
	}

	_, err := c.conn.Filters.SetSourceFilterSettings(req)
	if err != nil {
		log.Printf("OBS SetOpacity Error: %v", err)
	}
}

func (c *Client) RestartMedia(inputName string) {
	action := "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART"
	req := &mediainputs.TriggerMediaInputActionParams{
		InputName:   &inputName,
		MediaAction: &action,
	}
	_, err := c.conn.MediaInputs.TriggerMediaInputAction(req)
	if err != nil {
		log.Printf("OBS: Не вдалося перезапустити медіа %s: %v", inputName, err)
	}
}
