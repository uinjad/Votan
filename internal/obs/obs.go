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

	// Guards against overlapping fade animations racing each other.
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

func (c *Client) RestartMedia(inputName string) {
	action := "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART"
	req := &mediainputs.TriggerMediaInputActionParams{
		InputName:   &inputName,
		MediaAction: &action,
	}
	_, err := c.conn.MediaInputs.TriggerMediaInputAction(req)
	if err != nil {
		log.Printf("OBS: failed to restart media %s: %v", inputName, err)
	}
}

// nextFadeGen bumps the animation counter and returns its new value.
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
		log.Printf("OBS: source %s not found in scene %s: %v", sourceName, sceneName, err)
		return
	}

	enableReq := &sceneitems.SetSceneItemEnabledParams{
		SceneItemEnabled: &enabled,
		SceneItemId:      &resp.SceneItemId,
		SceneName:        &sceneName,
	}

	_, err = c.conn.SceneItems.SetSceneItemEnabled(enableReq)
	if err != nil {
		log.Printf("OBS: failed to toggle source %s: %v", sourceName, err)
	}
}

// FadeSourceOpacity bails out early if a newer fade or SetOpacity superseded it.
func (c *Client) FadeSourceOpacity(sourceName, filterName string, startOpacity, endOpacity float64, duration time.Duration) {
	// Pin this animation's generation.
	currentGen := c.nextFadeGen()

	steps := 20
	sleepTime := duration / time.Duration(steps)
	stepSize := (endOpacity - startOpacity) / float64(steps)
	current := startOpacity

	bTrue := true

	for i := 0; i <= steps; i++ {
		// If someone started another fade or called SetOpacity, stop.
		c.mu.Lock()
		if c.fadeGen != currentGen {
			c.mu.Unlock()
			// Generation changed: quietly kill this goroutine.
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
			log.Printf("OBS fade error: %v", err)
			return
		}

		current += stepSize
		time.Sleep(sleepTime)
	}
}

// SetOpacity sets opacity instantly and cancels any in-flight fades.
func (c *Client) SetOpacity(sourceName, filterName string, opacity float64) {
	// Bump the counter so any running FadeSourceOpacity stops immediately.
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
		log.Printf("OBS SetOpacity error: %v", err)
	}
}
