package obs

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/filters"
	"github.com/andreykaipov/goobs/api/requests/mediainputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"

	"Votan/internal/engine"
)

// Compile-time guarantee that Client satisfies the engine's presentation port.
var _ engine.Scene = (*Client)(nil)

// Client is an engine.Scene backed by a live OBS WebSocket connection.
type Client struct {
	conn *goobs.Client

	mu      sync.Mutex
	fadeGen int
}

func NewClient(addr, password string) (*Client, error) {
	c, err := goobs.New(addr, goobs.WithPassword(password))
	if err != nil {
		return nil, fmt.Errorf("obs: connect %q: %w", addr, err)
	}
	return &Client{conn: c}, nil
}

// Close disconnects from OBS. Safe to call on a nil-conn client.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Disconnect(); err != nil {
		return fmt.Errorf("obs: disconnect: %w", err)
	}
	return nil
}

func (c *Client) RestartMedia(inputName string) {
	action := "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART"
	_, err := c.conn.MediaInputs.TriggerMediaInputAction(&mediainputs.TriggerMediaInputActionParams{
		InputName:   &inputName,
		MediaAction: &action,
	})
	if err != nil {
		slog.Error("obs: restart media failed", "input", inputName, "err", err)
	}
}

// nextFadeGen bumps the fade generation so an in-flight fade can detect that it
// has been superseded and bail out.
func (c *Client) nextFadeGen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fadeGen++
	return c.fadeGen
}

func (c *Client) SetSourceEnabled(sceneName, sourceName string, enabled bool) {
	resp, err := c.conn.SceneItems.GetSceneItemId(&sceneitems.GetSceneItemIdParams{
		SceneName:  &sceneName,
		SourceName: &sourceName,
	})
	if err != nil {
		slog.Error("obs: source lookup failed", "scene", sceneName, "source", sourceName, "err", err)
		return
	}
	_, err = c.conn.SceneItems.SetSceneItemEnabled(&sceneitems.SetSceneItemEnabledParams{
		SceneItemEnabled: &enabled,
		SceneItemId:      &resp.SceneItemId,
		SceneName:        &sceneName,
	})
	if err != nil {
		slog.Error("obs: toggle source failed", "source", sourceName, "err", err)
	}
}

func (c *Client) FadeSourceOpacity(sourceName, filterName string, from, to float64, duration time.Duration) {
	gen := c.nextFadeGen()

	const steps = 20
	sleep := duration / steps
	delta := (to - from) / steps
	current := from
	overlay := true

	for i := 0; i <= steps; i++ {
		c.mu.Lock()
		superseded := c.fadeGen != gen
		c.mu.Unlock()
		if superseded {
			return // a newer fade or SetOpacity took over
		}
		_, err := c.conn.Filters.SetSourceFilterSettings(&filters.SetSourceFilterSettingsParams{
			SourceName:     &sourceName,
			FilterName:     &filterName,
			FilterSettings: map[string]interface{}{"opacity": current},
			Overlay:        &overlay,
		})
		if err != nil {
			slog.Error("obs: fade step failed", "source", sourceName, "err", err)
			return
		}
		current += delta
		time.Sleep(sleep)
	}
}

func (c *Client) SetOpacity(sourceName, filterName string, opacity float64) {
	c.nextFadeGen() // cancel any in-flight fade
	overlay := true
	_, err := c.conn.Filters.SetSourceFilterSettings(&filters.SetSourceFilterSettingsParams{
		SourceName:     &sourceName,
		FilterName:     &filterName,
		FilterSettings: map[string]interface{}{"opacity": opacity},
		Overlay:        &overlay,
	})
	if err != nil {
		slog.Error("obs: set opacity failed", "source", sourceName, "err", err)
	}
}
