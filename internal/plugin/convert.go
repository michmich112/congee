package plugin

import (
	"github.com/michmich112/congee/internal/nostr"
	sdk "github.com/michmich112/congee/sdk/plugin"
)

func nostrEventToSDK(ev *nostr.Event) sdk.Event {
	if ev == nil {
		return sdk.Event{}
	}
	tags := make([][]string, len(ev.Tags))
	for i, t := range ev.Tags {
		tags[i] = append([]string(nil), t...)
	}
	return sdk.Event{
		ID:        ev.ID,
		PubKey:    ev.PubKey,
		CreatedAt: ev.CreatedAt,
		Kind:      ev.Kind,
		Tags:      tags,
		Content:   ev.Content,
		Sig:       ev.Sig,
	}
}

func sdkEventToNostr(ev sdk.Event) *nostr.Event {
	tags := make([][]string, len(ev.Tags))
	for i, t := range ev.Tags {
		tags[i] = append([]string(nil), t...)
	}
	return &nostr.Event{
		ID:        ev.ID,
		PubKey:    ev.PubKey,
		CreatedAt: ev.CreatedAt,
		Kind:      ev.Kind,
		Tags:      tags,
		Content:   ev.Content,
		Sig:       ev.Sig,
	}
}

func nostrFilterToSDK(f nostr.Filter) sdk.Filter {
	tags := make(map[string][]string)
	for k, v := range f.Tag {
		name := stringsTrimHash(k)
		tags[name] = append([]string(nil), v...)
	}
	out := sdk.Filter{
		IDs:     append([]string(nil), f.IDs...),
		Authors: append([]string(nil), f.Authors...),
		Kinds:   append([]int(nil), f.Kinds...),
		Since:   f.Since,
		Until:   f.Until,
		Limit:   f.Limit,
		Search:  f.SearchText(),
		Tags:    tags,
	}
	return out
}

func sdkFilterToNostr(f sdk.Filter) nostr.Filter {
	tag := make(map[string][]string)
	for k, v := range f.Tags {
		key := k
		if len(key) == 1 {
			key = "#" + key
		}
		tag[key] = append([]string(nil), v...)
	}
	var search *string
	if f.Search != "" {
		s := f.Search
		search = &s
	}
	return nostr.Filter{
		IDs:     append([]string(nil), f.IDs...),
		Authors: append([]string(nil), f.Authors...),
		Kinds:   append([]int(nil), f.Kinds...),
		Since:   f.Since,
		Until:   f.Until,
		Limit:   f.Limit,
		Search:  search,
		Tag:     tag,
	}
}

func nostrFiltersToSDK(in []nostr.Filter) []sdk.Filter {
	out := make([]sdk.Filter, 0, len(in))
	for _, f := range in {
		out = append(out, nostrFilterToSDK(f))
	}
	return out
}

func sdkFiltersToNostr(in []sdk.Filter) []nostr.Filter {
	out := make([]nostr.Filter, 0, len(in))
	for _, f := range in {
		out = append(out, sdkFilterToNostr(f))
	}
	return out
}

func stringsTrimHash(k string) string {
	if len(k) == 2 && k[0] == '#' {
		return k[1:]
	}
	return k
}

func cloneEvent(ev *nostr.Event) *nostr.Event {
	if ev == nil {
		return nil
	}
	c := nostrEventToSDK(ev)
	return sdkEventToNostr(c)
}

func cloneFilters(in []nostr.Filter) []nostr.Filter {
	out := make([]nostr.Filter, 0, len(in))
	for _, f := range in {
		out = append(out, sdkFilterToNostr(nostrFilterToSDK(f)))
	}
	return out
}
