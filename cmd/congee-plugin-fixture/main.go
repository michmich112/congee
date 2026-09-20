package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	sdk "github.com/michmich112/congee/sdk/plugin"
)

type h struct {
	host sdk.Host
}

func (h *h) SetHost(host sdk.Host) { h.host = host }

func (h *h) Handshake(ctx context.Context, settings json.RawMessage) (*sdk.HandshakeResult, error) {
	_ = ctx
	_ = settings
	return &sdk.HandshakeResult{
		PluginID:            "fixture",
		Name:                "Fixture",
		Version:             "0.0.1",
		Capabilities:        []string{sdk.CapObserve, sdk.CapIntercept, sdk.CapIndexOwn, sdk.CapEventsRead},
		InterceptDeadlineMs: 200,
		Subscriptions: []sdk.TrafficSubscription{
			{MessageTypes: []string{"EVENT"}, Kinds: []int{1, 30402, 34560}, Observe: true, OnStoredEvent: true},
			{MessageTypes: []string{"REQ"}, Kinds: []int{30402, 34560}, ReqHasSearch: true, ReqTagNames: []string{"g"}, InterceptREQ: true, Observe: true},
		},
	}, nil
}

func (h *h) Health(ctx context.Context) (bool, string, error) {
	_ = ctx
	return true, "ok", nil
}

func (h *h) Observe(ctx context.Context, msg sdk.ObserveMessage) error {
	_ = ctx
	if os.Getenv("PLUGIN_SLOW_LISTEN") == "1" {
		time.Sleep(2 * time.Second)
	}
	_ = msg
	return nil
}

func (h *h) OnStoredEvent(ctx context.Context, ev sdk.Event, stored bool) error {
	_ = ctx
	_ = stored
	if os.Getenv("PLUGIN_SLOW_LISTEN") == "1" {
		time.Sleep(2 * time.Second)
	}
	_ = ev
	return nil
}

func (h *h) InterceptREQ(ctx context.Context, req sdk.Req) (*sdk.InterceptResult, error) {
	_ = ctx
	mode := os.Getenv("PLUGIN_INTERCEPT")
	switch mode {
	case "respond":
		ids := strings.Split(os.Getenv("PLUGIN_RESPOND_IDS"), ",")
		var clean []string
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" {
				clean = append(clean, id)
			}
		}
		return &sdk.InterceptResult{Action: sdk.InterceptRespond, EventIDs: clean}, nil
	case "reshape":
		if len(req.Filters) == 0 {
			return &sdk.InterceptResult{Action: sdk.InterceptPassthrough}, nil
		}
		f := req.Filters[0]
		f.Kinds = []int{30402}
		return &sdk.InterceptResult{Action: sdk.InterceptReshapeREQ, ReshapeFilters: []sdk.Filter{f}}, nil
	case "error":
		return nil, os.ErrInvalid
	default:
		return &sdk.InterceptResult{Action: sdk.InterceptPassthrough}, nil
	}
}

func (h *h) ApplySettings(ctx context.Context, settings json.RawMessage) ([]sdk.TrafficSubscription, error) {
	_ = ctx
	_ = settings
	hs, _ := h.Handshake(ctx, settings)
	return hs.Subscriptions, nil
}

func (h *h) AdminAction(ctx context.Context, name string, payload json.RawMessage) (json.RawMessage, error) {
	_ = ctx
	_ = payload
	return json.Marshal(map[string]string{"action": name, "ok": "true"})
}

func (h *h) Status(ctx context.Context) (*sdk.Status, error) {
	_ = ctx
	return &sdk.Status{Ready: true, JSON: json.RawMessage(`{"fixture":true}`)}, nil
}

func main() {
	ctx := context.Background()
	if err := sdk.Serve(ctx, &h{}); err != nil {
		os.Exit(1)
	}
}
